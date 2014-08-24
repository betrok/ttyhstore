package main

import (
	"os"
	"io"
	"fmt"
	"log"
	"sort"
	"time"
	"flag"
	"bytes"
	"strings"
	"net/http"
	"io/ioutil"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
)

var (
	store_root, prefix_root string
	special_dirs = []string{"versions", "libraries", "assets"}
	
	url = map[string] string {
		"versions":	"http://s3.amazonaws.com/Minecraft.Download/versions/",
	 
		"libs":		"https://libraries.minecraft.net/",
		"indexes":	"https://s3.amazonaws.com/Minecraft.Download/indexes/",
		"assets":	"http://resources.download.minecraft.net/",
	}
	
	os_list = []string{"linux", "windows", "osx"/*, "MS-DOS"*/}
	arch_list = []string{/*"3.14", "8", "16",*/ "32", "64"/*, "128"*/}
	
	custom_last = map[string] string {}
	ignore_list = map[string] bool {}
	
	clone = ""
	prefix = ""
	
	verbose, localCheck, cleanup bool
	
	checked = struct {
		libs, indexes, assets map[string] bool
	}{ map[string]bool{}, map[string]bool{}, map[string]bool{} }
	
	invalids bool
)

func main() {
	log.SetFlags(0)
	
	action, args := configure()
	
	switch action {
		case "cleanup":
			cleanup = true
			fallthrough
		
		case "collect":
			collectAll()
			if(cleanup) {
				if(invalids) {
					log.Println("Cleanup aborted in case of invalid cli")
				} else {
					clean()
				}
			}
			
		case "check":
			for _, cli := range(args) {
				part := strings.Split(cli, "/")
				var err error
				switch len(part) {
					case 0:
						log.Fatal("wtf?")
						
					case 1:
						_, err = checkCli(store_root + prefix + "/", cli)
						
					case 2:
						_, err = checkCli(store_root + part[0] + "/", part[1])
						
					default:
						log.Fatalf("Too many slashes in \"%s\"", cli)
				}
				if(err != nil) {
					log.Printf("Client \"%s\" check failed: %v\n", cli, err)
				}
				log.Println()
			}
			
		case "clone":
			log.Printf("Clone to prefix \"%s\"", prefix)
			for _, cli := range(args) {
				if err := cloneCli(store_root + prefix + "/", cli); err != nil {
					log.Fatalf("Clone version \"%s\" failed: %v", cli, err)
				}
			}
			
		case "help":
			flag.Usage()
	}
}

func configure() (action string, args []string) {
	store_root = os.Getenv("TTYH_STORE")
	
	var last, ignore string
	var help bool
	
	
	flag.BoolVar(&help, "help", false, "generated help sucks, overwrite it")
	flag.BoolVar(&verbose, "v", false, "")
	flag.BoolVar(&cleanup, "cleanup", false, "")
	flag.StringVar(&store_root, "root", store_root, "")
	flag.StringVar(&last, "last", "", "")
	flag.StringVar(&ignore, "ignore", "", "")
	flag.StringVar(&prefix, "prefix", "default", "")
	
	flag.Usage = func() { log.Printf(help_msg, os.Args[0]) }
	flag.Parse()
	
	if(len(store_root) == 0) { 
		log.Fatal("Srote root not defined.")
	}
	
	if(help) { return "help", flag.Args() }
	
	if(inSlice(prefix, special_dirs) || len(prefix) == 0) {
		log.Fatal("Passed prefix belongs to special directories")
	}
	
	for name, link := range(url) {
		if(link[len(link) - 1] != '/') {
			url[name] = link + "/"
		}
	}
	if(store_root[len(store_root) - 1] != '/') {
		store_root += "/"
	}
	
	if(len(ignore) != 0) {
		for _, item := range(strings.Split(ignore, ",")) {
			ignore_list[item] = true
		}
	}
	
	args = flag.Args()
	if(len(args) == 0) {
		return "collect", args
	}
	return args[0], args[1:]
}

func cloneCli(prefix_root, cli string) error {
	_, err := getFile(url["versions"]+ cli + "/" + cli + ".jar",
		prefix_root + cli + "/" + cli + ".jar")
	if(err != nil) { return err }
	
	_, err = getFile(url["versions"] + cli + "/" + cli + ".json",
		prefix_root + cli + "/" + cli + ".json")
	if(err != nil) { return err }
	
	_, err = checkCli(prefix_root, cli)
	return err
}

func collectAll() {
	dir, err := ioutil.ReadDir(store_root)
	if(err != nil) {
		log.Fatal("Can't read store_root directory", err)
	}
	plist := newPrefixList()
	for _, fi := range dir {
		if(!fi.IsDir() || inSlice(fi.Name(), special_dirs)) { continue }
		
		pinfo := collectPrefix(store_root + fi.Name() + "/")
		if(pinfo.Type != "hide") {
			plist.Prefixes[fi.Name()] = pinfo
		}
	}
	
	data, _ := json.MarshalIndent(plist, "", "  ")
	log.Println("Generated prefixes.json:")
	log.Println(string(data))
	
	fd, err := os.Create(store_root + "prefixes.json")
	if(err != nil) { log.Fatal("Create prefixes.json failed:", err) }
	fd.Write(data)
	fd.Close()
}

func collectPrefix(prefix_root string) PrefixInfo {
	var err error
	prefix := filepath.Base(prefix_root)
	
	log.Printf("\Joining prefix \"%s\"\n\n", prefix)
	
	if err := os.MkdirAll(prefix_root + "versions", os.ModeDir | 0755); err != nil {
		log.Fatal(err)
	}
	
	var pinfo PrefixInfo
	var fd *os.File
	if fd, err = os.Open(prefix_root + "prefix.json"); err == nil {
		decoder := json.NewDecoder(fd)
		err = decoder.Decode(&pinfo)
		fd.Close()
	}
	if(err != nil) { 
		log.Println("W: prefix.json read failed, use generic info\n")
		pinfo.Type = "public"
	}
	
	new_vers := newVersions()
	
	dir, err := ioutil.ReadDir(prefix_root)
	if(err != nil) {
		log.Fatal("Can't read prefix root directory", err)
	}
	
	for _, fi := range dir {
		if(!fi.IsDir() || inSlice(fi.Name(), special_dirs) ||
			ignore_list[prefix + "/" + fi.Name()]) { continue }
		
		vinfo, err := checkCli(prefix_root, fi.Name())
		if(err == nil) {
			new_vers.Versions = append(new_vers.Versions, &vinfo.VInfoMin)
			lt, ok := new_vers.latestTime[vinfo.Type]
			if(!ok || lt.Before(vinfo.Release)) {
				new_vers.Latest[vinfo.Type] = vinfo.Id
				new_vers.latestTime[vinfo.Type] = vinfo.Release
			}
		} else {
			invalids = true
			log.Printf("Client \"%s\" check failed: %v\n", fi.Name(), err)
		}
		log.Println()
	}
	sort.Sort(VersSlice(new_vers.Versions))
	for t, _ := range(new_vers.Latest) {
		custom, ok := custom_last[prefix + "/" + t]
		//TODO do we need to check whatever custom version is valid?
		if(ok) { new_vers.Latest[t] = custom }
	}
	
	data, _ := json.MarshalIndent(new_vers, "", "  ")
	log.Println("Generated version.json:")
	log.Println(string(data))
	
	fd, err = os.Create(prefix_root + "versions/versions.json")
	if(err != nil) { log.Fatal("Create versions.json failed:", err) }
	fd.Write(data)
	fd.Close()
	log.Printf("\nDone in prefix \"%s\"\n\n", prefix)
	
	return pinfo
}

func checkCli(prefix_root, version string) (vinfo *VInfoFull, err error) {
	vinfo = newVInfoFull()
	log.Printf("Checking cli \"%s\"...\n", version)
	var fd *os.File
	if fd, err = os.Open(prefix_root + version + "/" + version + ".json"); err == nil {
		decoder := json.NewDecoder(fd)
		err = decoder.Decode(vinfo)
		fd.Close()
	}
	if(err != nil) { return }
	data, _ := json.MarshalIndent(vinfo.VInfoMin, "", "  ")
	log.Println(string(data))
	
	if(len(vinfo.Arguments) == 0 || len(vinfo.MainClass) == 0) {
		err = fmt.Errorf("Arguments or mainClass not defined")
		return
	}
	
	if(vinfo.Id != version) {
		err = fmt.Errorf("Mismatched dir name & client id: \"%s\" != \"%s\"", version, vinfo.Id)
		return
	}
	log.Printf("%v.json: OK", version)
	
	vers_root := prefix_root + version + "/"
	
	if _, err = os.Stat(vers_root + version + ".jar"); err != nil {
		return
	}
	log.Printf("%v.jar: OK", version)
	if _, err = os.Stat(vers_root + version + "-tweaker.jar"); err != nil {
		log.Printf("W: Tweaker not found for \"%s\"", version)
	}
	
	err = checkLibs(vinfo.Libs)
	if(err != nil) { return }
	log.Println("Libs: OK")
	
	if(len(vinfo.Assets) != 0) {
		err = checkAssets(vinfo.Assets, !vinfo.CustomAssets)
		if(err != nil) { return }
		log.Println("Assets: OK")
	} else {
		log.Printf("W: No assets defined in \"%s\"\n", version)
	}
	if(err != nil) { return }
	
	if(vinfo.CustomFiles) {
		err = checkFiles(vers_root)
		if(err != nil) { return }
		log.Println("Files: OK")
	}
	
	log.Printf("Cli \"%s\" seems to be suitable", version)
	return
}

func checkLibs(libInfo []*LibInfo) error {
	log.Println("Checking libs...")
	path_list := make([]string, 0, 10)
	for _, lib := range(libInfo) {
		part := strings.Split(lib.Name, ":")
		if(len(part) != 3) { return fmt.Errorf("Unknown lib name format \"%s\"", lib.Name) }
		part[0] = strings.Replace(part[0], ".", "/", -1)
		
		if(lib.Natives == nil) {
			path_list = append(path_list,
				fmt.Sprintf("%s/%s/%s/%s-%s.jar", part[0], part[1], part[2], part[1], part[2]))
		} else {
			sub_dir := fmt.Sprintf("%s/%s/%s", part[0], part[1], part[2])
			
			var needers []string
			if(lib.Rules != nil) {
				needers = genNeeders(lib.Rules)
			} else {
				needers = os_list
			}
			
			for os, suffix := range(lib.Natives) {
				//unknown or disallowed os
				if(!inSlice(os, needers)) {
					if(!inSlice(os, os_list)) {
						log.Printf("W: Unknown os \"%s\" in natives", os)
					}
					continue
				}
				
				if(strings.Contains(suffix, "${arch}")) {
					for _, arch := range(arch_list) {
						path_list = append(path_list,
							fmt.Sprintf("%s/%s-%s-%s.jar", sub_dir, part[1], part[2],
								strings.Replace(suffix, "${arch}", arch, -1)))
					}
				} else {
					path_list = append(path_list,
						fmt.Sprintf("%s/%s-%s-%s.jar", sub_dir, part[1], part[2], suffix))
				}
			}
		}
	}
	for _, path := range(path_list) {
		if(checked.libs[filepath.Base(path)]) {
			if(verbose) {
				log.Printf("Lib \"%s\" already checked\n", filepath.Base(path))
			}
			continue
		}
		if err := getLib(path); err != nil { return err }
		checked.libs[filepath.Base(path)] = true
	}
	return nil
}

func genNeeders(rules []Rule) []string {
	ns := make([]string, 0, len(os_list))
	for _, rule := range(rules) {
		switch {
			case rule.Os == nil && rule.Action == "allow":
				ns = ns[0:len(os_list)]
				copy(ns, os_list)
				
			case rule.Action == "allow":
				ns = append(ns, rule.Os["name"])
				
			case rule.Action == "disallow":
				if _, ok := rule.Os["version"]; !ok {
					for i, os := range(ns) {
						if(os == rule.Os["name"]) {
							ns[i], ns = ns[len(ns)-1], ns[:len(ns)-1]
						}
					}
				}
				
			default:
				log.Printf("Can't handle unknown rule: %+v", rule)
		}
	}
	return ns
}

func getLib(path string) error {
	hash, err := readHashfile(store_root + "libraries/" + path + ".sha1")
	if(err != nil) {
		if(!os.IsNotExist(err)) {
			log.Printf("While reading hash file for \"%s\": %v", filepath.Base(path), err)
		}
		_, err = getFile(url["libs"] + path + ".sha1", store_root + "libraries/" + path + ".sha1")
		if(err != nil) { return err }
		hash, err = readHashfile(store_root + "libraries/" + path + ".sha1")
		if(err != nil) { return err }
	} else if(verbose) {
		log.Printf("Hash file for lib \"%s\" already exist\n", filepath.Base(path))
	}
	
	fhash, err := fileHash(store_root + "libraries/" + path)
	if(err == nil && bytes.Equal(hash, fhash)) {
		if(verbose) {
			log.Printf("Lib \"%s\" already exist\n", filepath.Base(path))
		}
		return nil
	}
	
	if(err == nil) {
		log.Printf("Hash sums mismatched to \"%s\": %s != %s. Regetting...",
		filepath.Base(path), hex.EncodeToString(hash), hex.EncodeToString(fhash))
	}
	_, err = getFile(url["libs"] + path, store_root + "libraries/" + path)
	if(err != nil) { return err }
	
	fhash, err = fileHash(store_root + "libraries/" + path)
	if(err != nil) { return fmt.Errorf("Unable to calculate hash sum: %v", err) }
	
	if(bytes.Equal(hash, fhash)) { return nil }
	
	return fmt.Errorf("Hash sums mismatched to \"%s\": %s != %s. Regetting change noting.",
		filepath.Base(path), hex.EncodeToString(hash), hex.EncodeToString(fhash))
}

func readHashfile(full_path string) ([]byte, error) {
	fd, err := os.Open(full_path)
	if(err != nil) { return nil, err }
	defer fd.Close()
	
	buf := make([]byte, 40)
	_, err = fd.Read(buf)
	if(err != nil) { return nil, err }
	
	hash := make([]byte, 20)
	_, err = hex.Decode(hash, buf)
	return hash, err
}

func parceIndex(path string) (*ObjectList, error) {
	fd, err := os.Open(path)
	if(err != nil) { return nil, err }
	
	list := newObjectList()
	decoder := json.NewDecoder(fd)
	err = decoder.Decode(list)
	
	fd.Close()
	return list, err
}

func checkFiles(vers_root string) (error) {
	log.Printf("Checking files for \"%s\"...\n", filepath.Base(vers_root))
	list, err := parceIndex(vers_root + "files.json")
	if(err != nil) { return err }
	
	//TODO check hash
	for name, a := range(list.Data) {
		fi, err := os.Stat(vers_root + "files/" + name)
		if(err == nil) { 
			if(fi.Size() == a.Size) {
				if(verbose) {
					log.Printf("File \"%s\" checked successfully\n", name)
				}
				continue
			}
			return fmt.Errorf("File \"%s\" size mismatch with definition\n", name, a.Hash)
		}
	}
	return nil
}

func checkAssets(version string, official bool) (err error) {
	log.Printf("Checking assets \"%s\"...\n", version)
	
	if(checked.indexes[version + ".json"]) {
		if(verbose) {
			log.Printf("Index \"%s\" already checked\n", version + ".json")
		}
		return nil
	}
	
	if err = os.MkdirAll(store_root + "assets/indexes/", os.ModeDir | 0755); err != nil { return }
	if err = os.MkdirAll(store_root + "assets/objects/", os.ModeDir | 0755); err != nil { return }
	
	list, err := parceIndex(store_root + "assets/indexes/" + version + ".json")
	if(err != nil) {
		if(os.IsNotExist(err) && official) {
			log.Printf("W: Assets index \"%s\" not found, downloading official version", version)
			_, err = getFile(url["indexes"] + version + ".json", store_root + "assets/indexes/" + version + ".json")
			if(err != nil) { return }
			list, err = parceIndex(version + ".json")
			if(err != nil) { return }
		} else { return }
	}
	
	//TODO check hash
	for name, a := range(list.Data) {
		if(checked.assets[a.Hash]) {
			if(verbose) {
				log.Printf("Already checked: \"%s\"(%s)\n", name, a.Hash)
			}
			continue
		}
		
		if(len(a.Hash) != 40 || a.Size <= 0) {
			return fmt.Errorf("Assets \"%s\"(%s) size or hash defined incorrect", name, a.Hash)
		}
		local_path := a.Hash[:2] + "/" + a.Hash
		fi, err := os.Stat(store_root + "assets/objects/" + local_path)
		if(err == nil) { 
			if(fi.Size() == a.Size) {
				if(verbose) {
					log.Printf("Exist: \"%s\"(%s)\n", name, a.Hash)
				}
				checked.assets[a.Hash] = true
				continue
			}
			log.Printf("Assets \"%s\"(%s) local file size mismatch with definition, regeting\n", name, a.Hash)
		}
		
		size, err := getFile(url["assets"] + local_path, store_root + "assets/objects/" + local_path)
		if(err != nil) { return err }
		
		if(size != a.Size) {
			return fmt.Errorf("Downloaded asset \"%s\"(%s) size mismatch with definition", name, a.Hash)
		}
		checked.assets[a.Hash] = true
	}
	
	checked.indexes[version + ".json"] = true
	return
}

//TODO remove empty dirs
func clean() {
	log.Println("Cleaning up...\n")
	indexes_root := store_root + "assets/indexes/"
	dir, err := ioutil.ReadDir(indexes_root)
	if(err != nil) {
		log.Fatal("Can't read assets indexes directory", err)
	}
	for _, fi := range dir {
		if(fi.IsDir() || checked.indexes[fi.Name()]) { continue }
		err = os.Remove(indexes_root + fi.Name())
		if(err != nil) { log.Fatal("Cleanup failed:", err) }
		if(verbose) {
			log.Printf("Index \"%s\" deleted", fi.Name())
		}
	}
	
	libs_root := store_root + "libraries/"
	filepath.Walk(libs_root, func(path string, info os.FileInfo, err error)(error) {
			if(err != nil) { log.Println("While walking over libraries:", err) }
			if(info.IsDir()) {
				return nil
			} else if(!checked.libs[strings.TrimSuffix(filepath.Base(path), ".sha1")]) {
				err = os.Remove(path)
				if(err != nil) {
					log.Fatal(err)
				} else if(verbose) {
					log.Printf("In libs: \"%s\" deleted", info.Name())
				}
			}
			return nil
		})
	
	filepath.Walk(store_root + "assets/objects/", func(path string, info os.FileInfo, err error)(error) {
		if(err != nil) { log.Println("While walking over libraries:", err) }
		if(info.IsDir()) {
			return nil
		} else if(!checked.assets[filepath.Base(path)]) {
			err = os.Remove(path)
			if(err != nil) {
				log.Fatal(err)
			} else if(verbose) {
				log.Printf("In assets: \"%s\" deleted", info.Name())
			}
		}
		return nil
	})
	
	
	log.Println("Cleanup finished")
}

func rmEmptyDirs(path string) (err error) {
	flist, err := ioutil.ReadDir(path)
	for len(flist) == 0 && err == nil {
		err = os.Remove(path)
		path = filepath.Dir(path)
		flist, _ = ioutil.ReadDir(path)
	}
	return err
}

func getFile(url, dest_path string) (int64, error) {
	log.Printf("Getting file \"%s\"...", filepath.Base(dest_path))
	if err := os.MkdirAll(filepath.Dir(dest_path), os.ModeDir | 0755); err != nil {
		return 0, err
	}
	resp, err := http.Get(url)
	if(err != nil) { return 0, err }
	defer resp.Body.Close()
	if(resp.StatusCode != 200) {
		return 0, fmt.Errorf("Loading file \"%s\" failed with status \"%s\"", url, resp.Status)
	}
	log.Printf("%s (%s)", resp.Status, readableSize(float64(resp.ContentLength)))
	start := time.Now()
	
	fd, err := os.Create(dest_path)
	if(err != nil) { return 0, err }
	defer fd.Close()
	
	size, err := io.Copy(fd, resp.Body)
	if(err != nil) { return 0, err }
	
	delta := time.Now().Sub(start)
	log.Printf("Done in %v, %s/s", delta, readableSize(float64(size) * float64(time.Second) / float64(delta)))
	return size, err
}

func readableSize(in float64) string {
	var suffix = []string{"b", "kB", "MB", "GB", "TB", "PB"}
	sit := 0
	for in > 1024 {
		in /= 1024
		sit++
	}
	if(sit >= len(suffix)) { return "over9000" }
	return fmt.Sprintf("%.2f %s", in, suffix[sit])
}

func fileHash(path string) ([]byte, error) {
	fd, err := os.Open(path)
	if(err != nil) { return nil, err }
	defer fd.Close()
	
	h := sha1.New()
	io.Copy(h, fd)
	return h.Sum(nil), nil
}

func inSlice(val string, sli []string) bool {
	for _, t := range(sli) {
		if(val == t) { return true }
	}
	return false
}
