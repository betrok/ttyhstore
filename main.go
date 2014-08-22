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
	
	versions_url = "http://s3.amazonaws.com/Minecraft.Download/versions/"
	
	libs_url = "https://libraries.minecraft.net/"
	aindex_url = "https://s3.amazonaws.com/Minecraft.Download/indexes/"
	assets_url = "http://resources.download.minecraft.net/"
	
	os_list = []string{"linux", "windows", "osx"/*, "MS-DOS"*/}
	arch_list = []string{/*"3.14", "8", "16",*/ "32", "64"/*, "128"*/}
	custom_last = ""
	ignore = ""
	clone = ""
	vers_prefix = ""
	
	verbose, localCheck bool
)

func main() {
	log.SetFlags(0)
	
	store_root = os.Getenv("TTYH_STORE")
	if(len(store_root) == 0) { store_root = "./" }
	
	flag.BoolVar(&verbose, "v", false, `
		be verbose
		`)
	
	flag.BoolVar(&localCheck, "l", false, `
		Check only passed in args clients, don't rewrite version.json
		`)
	
	flag.StringVar(&store_root, "root", store_root, `
		Root of file storage(may be set by $TTYH_STORE env variable)
		`)
	
	flag.StringVar(&custom_last, "last", "", `
		--last=<type1>:<version1>[,<type2>:<version2>][...]
		Overwrite latest versions in versions.json manualy.
		Default choise based on releaseTime in <version>.json
		`)
	
	flag.StringVar(&ignore, "ignore", "", `
		--ignore=<version1>[,<version2>][...]
		Don't check specified versions.
		`)

	flag.StringVar(&clone, "clone", "", `
		--clone=<version1>[,<version2>][...]
		Load version[s] from official repo.
		`)
	
	flag.StringVar(&vers_prefix, "prefix", "default", `
		Set version prefix for work. Uservull for cursom clients trees.
		All prefixes have common libs & assets directorys.
		`)
	
	flag.Parse()
	
	if(len(vers_prefix) == 0) {
		prefix_root = store_root
		log.Println("No prefix defined, working in store_root")
	} else {
		prefix_root = store_root + "/" + vers_prefix
		log.Printf("Working in prefix \"%s\"\n\n", vers_prefix)
	}
	
	if(localCheck) {
		for _, cli := range(flag.Args()) {
			checkCli(cli)
			log.Println()
		}
		return
	}
	
	if(len(clone) != 0) {
		for _, cli := range(strings.Split(clone, ",")) {
			if err := cloneCli(cli); err != nil {
				log.Fatalf("Clone version \"%s\" failed: %v", cli, err)
			}
		}
		return
	}
	
	checkAll()
}

func cloneCli(cli string) error {
	_, err := getFile(versions_url+ cli + "/" + cli + ".jar",
		prefix_root + "/" + cli + "/" + cli + ".jar")
	if(err != nil) { return err }
	
	_, err = getFile(versions_url + cli + "/" + cli + ".json",
		prefix_root + "/" + cli + "/" + cli + ".json")
	if(err != nil) { return err }
	
	_, err = checkCli(cli)
	return err
}

func checkAll() {
	new_vers := newVersions()
	
	if err := os.MkdirAll(prefix_root + "/versions", os.ModeDir | 0755); err != nil {
		log.Fatal(err)
	}
	dir, err := ioutil.ReadDir(prefix_root)
	if(err != nil) {
		log.Fatal("Cann't read prefix_root directory", err)
	}
	
	for _, fi := range dir {
		if(!fi.IsDir() || inSlice(fi.Name(), special_dirs) || 
			inSlice(fi.Name(), strings.Split(ignore, ","))) { continue }
		
		vinfo, err := checkCli(fi.Name())
		if(err == nil) {
			new_vers.Versions = append(new_vers.Versions, &vinfo.VInfoMin)
			lt, ok := new_vers.latestTime[vinfo.Type]
			if(!ok || lt.Before(vinfo.Release)) {
				new_vers.Latest[vinfo.Type] = vinfo.Id
				new_vers.latestTime[vinfo.Type] = vinfo.Release
			}
		} else {
			log.Printf("Client \"%s\" check failed: %v\n", fi.Name(), err)
		}
		log.Println()
	}
	sort.Sort(VersSlice(new_vers.Versions))
	if(len(custom_last) != 0) {
		for _, pair := range(strings.Split(custom_last, ",")) {
			t := strings.Split(pair, ":")
			if(len(t) != 2) { log.Fatal("Bad \"--last\" parametr") }
			new_vers.Latest[t[0]] = t[1]
		}
	}
	
	data, _ := json.MarshalIndent(new_vers, "", "  ")
	log.Println("Generated version.json:")
	log.Println(string(data))
	
	fd, err := os.Create(prefix_root + "/versions/versions.json")
	if(err != nil) { log.Fatal("Create versions.json failed:", err) }
	fd.Write(data)
	fd.Close()
}

func checkCli(version string) (vinfo *VInfoFull, err error) {
	vinfo = newVInfoFull()
	log.Printf("Checking cli \"%s\"...\n", version)
	var fd *os.File
	if fd, err = os.Open(prefix_root + "/" + version + "/" + version + ".json"); err == nil {
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
	
	if _, err = os.Stat(prefix_root + "/" + version + "/" + version + ".jar"); err != nil {
		return
	}
	log.Printf("%v.jar: OK", version)
	if _, err = os.Stat(prefix_root + "/" + version + "/" + version + "-tweaker.jar"); err != nil {
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
	
	if(err == nil) { log.Printf("Cli \"%s\" seems to be suitable", version) }
	return
}

func inSlice(val string, sli []string) bool {
	for _, t := range(sli) {
		if(val == t) { return true }
	}
	return false
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
		if err := getLib(path); err != nil { return err }
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
				log.Printf("Cann't handle unknown rule: %+v", rule)
		}
	}
	return ns
}

func getLib(path string) error {
	hash, err := readHashfile(store_root + "/libraries/" + path + ".sha1")
	if(err != nil) {
		if(!os.IsNotExist(err)) {
			log.Printf("While reading hashfile for \"%s\": %v", filepath.Base(path), err)
		}
		_, err = getFile(libs_url + path + ".sha1", store_root + "/libraries/" + path + ".sha1")
		if(err != nil) { return err }
		hash, err = readHashfile(store_root + "/libraries/" + path + ".sha1")
		if(err != nil) { return err }
	} else if(verbose) {
		log.Printf("Hash file for lib \"%s\" already exist\n", filepath.Base(path))
	}
	
	fhash, err := fileHash(store_root + "/libraries/" + path)
	if(err == nil && bytes.Equal(hash, fhash)) {
		if(verbose) {
			log.Printf("Lib \"%s\" already exist\n", filepath.Base(path))
		}
		return nil
	}
	
	if(err == nil) {
		log.Printf("Hashsums mismatched to \"%s\": %s != %s. Regetting...",
		filepath.Base(path), hex.EncodeToString(hash), hex.EncodeToString(fhash))
	}
	_, err = getFile(libs_url + path, store_root + "/libraries/" + path)
	if(err != nil) { return err }
	
	fhash, err = fileHash(store_root + "/libraries/" + path)
	if(err != nil) { return fmt.Errorf("Unable to calculate hashsum: %v", err) }
	
	if(bytes.Equal(hash, fhash)) { return nil }
	
	return fmt.Errorf("Hashsums mismatched to \"%s\": %s != %s. Regetting change noting.",
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

func parceIndex(version string) (*AssetsList, error) {
	fd, err := os.Open(store_root + "/assets/indexes/" + version + ".json")
	if(err != nil) { return nil, err }
	
	list := newAssetsList()
	decoder := json.NewDecoder(fd)
	err = decoder.Decode(list)
	fd.Close()
	return list, err
}

func checkAssets(version string, official bool) (err error) {
	log.Printf("Checking assets \"%s\"...\n", version)
	if err = os.MkdirAll(store_root + "/assets/indexes/", os.ModeDir | 0755); err != nil { return }
	if err = os.MkdirAll(store_root + "/assets/objects/", os.ModeDir | 0755); err != nil { return }
	
	list, err := parceIndex(version)
	if(err != nil) {
		if(os.IsNotExist(err) && official) {
			log.Printf("W: Assets index \"%s\" not found, downloading official version", version)
			_, err = getFile(aindex_url + version + ".json", store_root + "/assets/indexes/" + version + ".json")
			if(err != nil) { return }
			list, err = parceIndex(version)
			if(err != nil) { return }
		} else { return }
	}
	
	//TODO check hash
	for name, a := range(list.Data) {
		if(len(a.Hash) != 40 || a.Size <= 0) {
			return fmt.Errorf("Assets \"%s\"(%s) size or hash defined incorrect", name, a.Hash)
		}
		local_path := a.Hash[:2] + "/" + a.Hash
		fi, err := os.Stat(store_root + "/assets/objects/" + local_path)
		if(err == nil) { 
			if(fi.Size() == a.Size) {
				if(verbose) {
					log.Printf("Asset \"%s\"(%s) already exist\n", name, a.Hash)
				}
				continue
			}
			log.Printf("Assets \"%s\"(%s) local file size mismatch with definition, regeting\n", name, a.Hash)
		}
		
		size, err := getFile(assets_url + local_path, store_root + "/assets/objects/" + local_path)
		if(err != nil) { return err }
		
		if(size != a.Size) {
			return fmt.Errorf("Downloaded asset \"%s\"(%s) size mismatch with definition", name, a.Hash)
		}
	}
	
	return
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
