package main

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	storeRoot   string
	specialDirs = []string{"libraries", "assets"}

	url = struct {
		Versions, Manifest, Libraries, Indexes, Assets string
	}{
		Manifest:  "https://launchermeta.mojang.com/mc/game/version_manifest.json",
		Libraries: "https://libraries.minecraft.net/",
		Indexes:   "https://s3.amazonaws.com/Minecraft.Download/indexes/",
		Assets:    "https://resources.download.minecraft.net/",
	}

	osList   = []string{"linux", "windows", "osx" /*, "MS-DOS"*/}
	archList = []string{ /*"3.14", "8", "16",*/ "32", "64" /*, "128"*/}

	customLast   = map[string]string{}
	ignoreList   = map[string]bool{}
	libOverwrite []*regexp.Regexp

	prefix string

	verbose, cleanup, replace bool

	checked = struct {
		libs            map[string]FInfo
		indexes, assets map[string]bool
	}{map[string]FInfo{}, map[string]bool{}, map[string]bool{}}

	invalids bool
)

const overwriteFile = "overwrite.list"

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout)

	action, args := configure()

	switch action {
	case "cleanup":
		cleanup = true
		fallthrough

	case "collect":
		collectAll()
		if cleanup {
			if invalids {
				log.Println("Cleanup aborted in case of invalid cli")
			} else {
				clean()
			}
		}

	case "check":
		for _, cli := range args {
			var err error
			switch strings.Count(cli, "/") {
			case 0:
				_, err = checkCli(storeRoot+prefix+"/"+cli+"/", false)

			case 1:
				_, err = checkCli(storeRoot+cli+"/", false)

			default:
				log.Fatalf("Too many slashes in \"%s\"", cli)
			}
			if err != nil {
				log.Printf("Client \"%s\" check failed: %v\n", cli, err)
			}
			log.Println()
		}

	case "clone":
		log.Printf("Clone to prefix \"%s\"", prefix)
		for _, cli := range args {
			if err := cloneCli(storeRoot+prefix+"/", cli); err != nil {
				log.Fatalf("Clone version \"%s\" failed: %v", cli, err)
			}
		}

	default:
		flag.Usage()
	}
}

func configure() (action string, args []string) {
	storeRoot = os.Getenv("TTYH_STORE")

	var last, ignore string
	var help bool

	flag.BoolVar(&help, "help", false, "generated help sucks, overwrite it")
	flag.BoolVar(&verbose, "v", false, "")
	flag.BoolVar(&cleanup, "cleanup", false, "")
	flag.BoolVar(&replace, "replace", false, "")
	flag.StringVar(&storeRoot, "root", storeRoot, "")
	flag.StringVar(&last, "last", "", "")
	flag.StringVar(&ignore, "ignore", "", "")
	flag.StringVar(&prefix, "prefix", "default", "")

	flag.Usage = func() { log.Printf(helpMessage, os.Args[0]) }
	flag.Parse()

	if len(storeRoot) == 0 {
		log.Println("Srote root not defined.")
		help = true
	}

	if help {
		return "help", flag.Args()
	}

	if inSlice(prefix, specialDirs) || len(prefix) == 0 {
		log.Fatal("Passed prefix belongs to special directories")
	}

	if storeRoot[len(storeRoot)-1] != '/' {
		storeRoot += "/"
	}

	if len(last) != 0 {
		for _, t := range strings.Split(last, ",") {
			part := strings.Split(last, ":")
			if len(part) != 2 {
				log.Printf("Invalid --last format in \"%s\"", t)
				return "help", nil
			}
			customLast[part[0]] = part[1]
		}
	}

	if len(ignore) != 0 {
		for _, item := range strings.Split(ignore, ",") {
			ignoreList[item] = true
		}
	}

	err := readLibOverwrite()
	if err != nil {
		log.Fatalf("failed to prerare lib owerwrite: %v", err)
	}

	args = flag.Args()
	if len(args) == 0 {
		return "collect", args
	}
	return args[0], args[1:]
}

func readLibOverwrite() error {
	fd, err := os.Open(storeRoot + "libraries/" + overwriteFile)
	switch {
	case err == nil:

	case os.IsNotExist(err):
		// whatever
		return nil

	default:
		return err
	}

	defer fd.Close()

	sc := bufio.NewScanner(fd)
	for sc.Scan() {
		if sc.Text() == "" {
			continue
		}
		r, err := regexp.Compile(sc.Text())
		if err != nil {
			return fmt.Errorf("%v : %v", sc.Text(), err)
		}
		libOverwrite = append(libOverwrite, r)
	}
	return sc.Err()
}

func cloneCli(prefixRoot, cli string) error {
	manifestPath := storeRoot + "version_manifest.json"
	err := getFile(&Download{URL: url.Manifest}, manifestPath)
	if err != nil {
		return err
	}

	fd, err := os.Open(manifestPath)
	if err != nil {
		return err
	}
	defer fd.Close()

	var manifest VersionManifest
	err = json.NewDecoder(fd).Decode(&manifest)
	if err != nil {
		return fmt.Errorf("failed to decode version manifest: %v", err)
	}

	var version VInfoMin
	for _, t := range manifest.Versions {
		if t.Id == cli {
			version = t
			break
		}
	}
	if version.Id == "" {
		return fmt.Errorf("requseted version not found in manifest")
	}

	err = getFile(&Download{URL: version.URL}, prefixRoot+cli+"/"+cli+".json")
	if err != nil {
		return err
	}

	_, err = checkCli(prefixRoot+cli+"/", true)
	return err
}

func collectAll() {
	dir, err := ioutil.ReadDir(storeRoot)
	if err != nil {
		log.Fatal("Can't read storeRoot directory", err)
	}
	plist := NewPrefixList()
	for _, fi := range dir {
		if !fi.IsDir() || inSlice(fi.Name(), specialDirs) || strings.HasPrefix(fi.Name(), ".") {
			continue
		}

		pinfo := collectPrefix(storeRoot + fi.Name() + "/")
		if pinfo.Type != "hidden" {
			plist.Prefixes[fi.Name()] = pinfo
		}
	}

	data, _ := json.MarshalIndent(plist, "", "  ")
	log.Println("Generated prefixes.json:")
	log.Println(string(data))

	fd, err := os.Create(storeRoot + "prefixes.json")
	if err != nil {
		log.Fatalf("Failed to create prefixes.json: %v", err)
	}
	_, err = fd.Write(data)
	if err != nil {
		log.Fatalf("Failed to write prefixes.json: %v", err)
	}
	err = fd.Close()
	if err != nil {
		log.Fatalf("Failed to write prefixes.json: %v", err)
	}
}

func collectPrefix(prefixRoot string) PrefixInfo {
	var err error
	name := filepath.Base(prefixRoot)

	log.Printf("\nJoining prefix \"%s\"\n\n", name)

	if err := os.MkdirAll(prefixRoot+"versions", os.ModeDir|0755); err != nil {
		log.Fatal(err)
	}

	var pInfo PrefixInfoExt
	var fd *os.File
	if fd, err = os.Open(prefixRoot + "prefix.json"); err == nil {
		decoder := json.NewDecoder(fd)
		err = decoder.Decode(&pInfo)
		_ = fd.Close()

		for t, v := range pInfo.Latest {
			fullType := name + "/" + t
			if _, ok := customLast[fullType]; !ok {
				customLast[fullType] = v
			}
		}
	} else {
		log.Println("W: prefix.json read failed, use generic info\n")
		pInfo.Type = "public"
	}

	prefix := NewPrefix()

	dir, err := ioutil.ReadDir(prefixRoot)
	if err != nil {
		log.Fatal("Can't read prefix root directory", err)
	}

	for _, fi := range dir {
		if !fi.IsDir() || fi.Name() == "versions" ||
			ignoreList[name+"/"+fi.Name()] {
			continue
		}

		vInfo, err := checkCli(prefixRoot+fi.Name()+"/", false)
		if err == nil {
			prefix.Versions = append(prefix.Versions, &vInfo.VInfoMin)
			lt, ok := prefix.latestTime[vInfo.Type]
			if !ok || lt.Before(vInfo.Release) {
				prefix.Latest[vInfo.Type] = vInfo.Id
				prefix.latestTime[vInfo.Type] = vInfo.Release
			}
		} else {
			invalids = true
			log.Fatalf("Client \"%s\" check failed: %v\n", fi.Name(), err)
		}
		log.Println()
	}

	sort.Sort(VersionSlice(prefix.Versions))

	for t := range prefix.Latest {
		custom, ok := customLast[name+"/"+t]
		if ok {
			valid := false
			for _, version := range prefix.Versions {
				if version.Id == custom {
					if version.Type != t {
						log.Fatalf("In custom latest: mismatched client types for \"%s\"",
							name+"/"+t)
					}
					valid = true
					break
				}
			}
			if !valid {
				log.Fatalf("Custom latest for \"%s\" isn't consistent cli", name+"/"+t)
			}

			prefix.Latest[t] = custom
		}
	}

	data, _ := json.MarshalIndent(prefix, "", "  ")
	log.Println("Generated version.json:")
	log.Println(string(data))

	fd, err = os.Create(prefixRoot + "versions/versions.json")
	if err != nil {
		log.Fatal("Create versions.json failed:", err)
	}
	_, err = fd.Write(data)
	if err != nil {
		log.Fatal("Create versions.json failed:", err)
	}
	err = fd.Close()
	if err != nil {
		log.Fatal("Create versions.json failed:", err)
	}
	log.Printf("\nDone in prefix \"%s\"\n\n", name)

	return pInfo.PrefixInfo
}

func checkCli(versionRoot string, downloadJar bool) (*VInfoFull, error) {
	version := filepath.Base(versionRoot)

	log.Printf("Checking cli \"%s\"...\n", version)

	var (
		fd   *os.File
		err  error
		info VInfoFull
	)
	if fd, err = os.Open(versionRoot + version + ".json"); err == nil {
		err = json.NewDecoder(fd).Decode(&info)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %v.json: %v", version+".json")
		}
		_ = fd.Close()
	}
	if err != nil {
		return nil, err
	}

	if info.Id != version {
		return nil, fmt.Errorf("mismatched dir name & client id: \"%s\" != \"%s\"\n", version, info.Id)
	}

	log.Printf("%v.json: OK", version)

	var files FilesInfo

	jarPath := versionRoot + version + ".jar"
	jarInfo := &info.Downloads.Client

	if downloadJar {
		err = getFile(jarInfo, jarPath)
		if err != nil {
			return nil, err
		}
	}

	files.Main, err = getFInfo(jarPath)
	if err != nil {
		return nil, err
	}

	if !jarInfo.Match(files.Main) {
		return nil, fmt.Errorf("%s does not match expectations", version+".jar")
	}

	log.Printf("%v.jar: OK", version)

	if len(info.Assets) != 0 {
		err = checkAssets(info.Assets, &info.AssetIndex)
		if err != nil {
			return nil, err
		}
		log.Println("Assets: OK")
	} else {
		log.Printf("W: No assets defined for \"%s\"\n", version)
	}

	files.Files, err = collectCustoms(versionRoot)
	switch {
	case err == nil:
		log.Println("Files: OK")

	case os.IsNotExist(err):
		log.Println("Files aren't present")

	default:
		return nil, err
	}

	files.Libs, err = checkLibs(info.Libs)
	if err != nil {
		return nil, err
	}

	log.Println("Libraries: OK")

	data, _ := json.MarshalIndent(files, "", "  ")
	fd, err = os.Create(versionRoot + "data.json")
	if err != nil {
		log.Fatalf("failed to create data.json: %v", err)
	}
	_, err = fd.Write(data)
	if err != nil {
		log.Fatalf("failed to write to data.json: %v", err)
	}
	_ = fd.Close()

	log.Printf("Cli \"%s\" seems to be suitable", version)
	return &info, nil
}

func checkLibs(libInfo []LibInfo) (FIndex, error) {
	log.Println("Checking libs...")
	index := make(FIndex)

	for _, lib := range libInfo {
		if lib.Downloads != nil {
			if lib.Downloads.Artifact != (LibDownload{}) {
				err := checkLib(&lib.Downloads.Artifact, index)
				if err != nil {
					return nil, err
				}
			}
			for _, class := range lib.Downloads.Classifiers {
				err := checkLib(&class, index)
				if err != nil {
					return nil, err
				}
			}
		} else {
			err := checkLibOld(&lib, index)
			if err != nil {
				return nil, err
			}
		}
	}
	return index, nil
}

func checkLibOverwrite(path string) bool {
	for _, r := range libOverwrite {
		if r.MatchString(path) {
			return true
		}
	}
	return false
}

func checkLib(dl *LibDownload, index FIndex) error {
	if info, ok := checked.libs[dl.Path]; ok {
		if !dl.Match(info) {
			if checkLibOverwrite(dl.Path) {
				if verbose {
					log.Printf("Lib \"%s\" already checked\n", dl.Path)
				}
				index[dl.Path] = info
				return nil
			}

			return fmt.Errorf("lib %v was already checked but has different expectations now", dl.Path)
		}
		if verbose {
			log.Printf("Lib \"%s\" already checked\n", dl.Path)
		}
		index[dl.Path] = info
		return nil
	}

	fullPath := storeRoot + "libraries/" + dl.Path

	info, err := getFInfo(fullPath)
	switch {
	case err == nil:

		switch {
		case dl.Match(info) || checkLibOverwrite(dl.Path):
			dl.SHA1 = info.Hash
			dl.Size = info.Size

		case replace:
			err = getFile(&dl.Download, storeRoot+"libraries/"+dl.Path)
			if err != nil {
				return err
			}

		default:
			return fmt.Errorf("existing lib %v does not match expectations", dl.Path)
		}

	case os.IsNotExist(err):
		err = getFile(&dl.Download, storeRoot+"libraries/"+dl.Path)
		if err != nil {
			return err
		}

	default:
		return err
	}

	index[dl.Path] = dl.ToFInfo()
	checked.libs[dl.Path] = dl.ToFInfo()

	return nil
}

func checkLibOld(lib *LibInfo, index FIndex) error {
	pathList := make([]string, 0, 10)

	part := strings.Split(lib.Name, ":")
	if len(part) != 3 {
		return fmt.Errorf("unknown lib name format \"%s\"", lib.Name)
	}
	part[0] = strings.Replace(part[0], ".", "/", -1)

	if lib.Natives == nil {
		pathList = append(pathList,
			fmt.Sprintf("%s/%s/%s/%s-%s.jar", part[0], part[1], part[2], part[1], part[2]))
	} else {
		subDir := fmt.Sprintf("%s/%s/%s", part[0], part[1], part[2])

		var needers []string
		if lib.Rules != nil {
			needers = genNeeders(lib.Rules)
		} else {
			needers = osList
		}

		for os, suffix := range lib.Natives {
			//unknown or disallowed os
			if !inSlice(os, needers) {
				if !inSlice(os, osList) {
					log.Printf("W: Unknown os \"%s\" in natives", os)
				}
				continue
			}

			if strings.Contains(suffix, "${arch}") {
				for _, arch := range archList {
					pathList = append(pathList,
						fmt.Sprintf("%s/%s-%s-%s.jar", subDir, part[1], part[2],
							strings.Replace(suffix, "${arch}", arch, -1)))
				}
			} else {
				pathList = append(pathList,
					fmt.Sprintf("%s/%s-%s-%s.jar", subDir, part[1], part[2], suffix))
			}
		}
	}

	for _, path := range pathList {
		info, ok := checked.libs[path]
		if ok {
			if verbose {
				log.Printf("Lib \"%s\" already checked\n", filepath.Base(path))
			}
		} else {
			baseURL := url.Libraries
			if len(lib.Url) > 0 {
				baseURL = lib.Url
			}
			info, err := getLibOld(path, baseURL)
			if err != nil {
				return err
			}
			checked.libs[path] = info
		}
		index[path] = info
	}

	return nil
}

func genNeeders(rules []Rule) []string {
	ns := make([]string, 0, len(osList))
	for _, rule := range rules {
		switch {
		case rule.Os == OsRule{} && rule.Action == "allow":
			ns = ns[0:len(osList)]
			copy(ns, osList)

		case rule.Action == "allow":
			ns = append(ns, rule.Os.Name)

		case rule.Action == "disallow":
			if rule.Os.Version == "" && rule.Os.Arch == "" {
				for i, os := range ns {
					if os == rule.Os.Name {
						ns[i], ns = ns[len(ns)-1], ns[:len(ns)-1]
					}
				}
			}

		default:
			log.Fatalf("Can't handle unknown rule: %+v", rule)
		}
	}
	return ns
}

func getLibOld(path, baseUrl string) (obj FInfo, err error) {
	fullPath := storeRoot + "libraries/" + path
	obj.Hash, err = readHashFile(fullPath + ".sha1")
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("While reading hash file for \"%s\": %v", filepath.Base(path), err)
		}
		err = getFile(&Download{
			URL: baseUrl + path + ".sha1",
		}, fullPath+".sha1")
		if err != nil {
			return
		}
		obj.Hash, err = readHashFile(fullPath + ".sha1")
		if err != nil {
			return
		}
	} else if verbose {
		log.Printf("Hash file for lib \"%s\" already exist\n", filepath.Base(path))
	}

	err = checkHash(fullPath, obj.Hash)
	if err == nil {
		if verbose {
			log.Printf("Lib \"%s\" already exist\n", filepath.Base(path))
		}
		fi, _ := os.Stat(fullPath)
		obj.Size = fi.Size()
		return
	}

	if !os.IsNotExist(err) {
		log.Printf("%v. Regetting...", err)
	}

	err = getFile(&Download{
		URL: baseUrl + path + ".sha1",
	}, fullPath+".sha1")
	if err != nil {
		return
	}
	obj.Hash, err = readHashFile(fullPath + ".sha1")
	if err != nil {
		return
	}

	dl := Download{
		URL:  baseUrl + path,
		SHA1: obj.Hash,
	}
	err = getFile(&dl, fullPath)
	if err != nil {
		return
	}

	return dl.ToFInfo(), nil
}

func readHashFile(full_path string) (string, error) {
	fd, err := os.Open(full_path)
	if err != nil {
		return "", err
	}
	defer fd.Close()

	buf := make([]byte, 40)
	_, err = fd.Read(buf)
	return string(buf), err
}

func parseIndex(path string) (*ObjectList, error) {
	fd, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	list := NewObjectList()
	decoder := json.NewDecoder(fd)
	err = decoder.Decode(list)

	_ = fd.Close()
	return list, err
}

func collectCustoms(vers_root string) (cust *Customs, err error) {
	log.Printf("Collecting files for \"%s\"...\n", filepath.Base(vers_root))

	cust = NewCustoms()

	root := vers_root + "files/"
	if _, err := os.Stat(root); err != nil {
		return nil, err
	}
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("While walking over files: %v", err)
		}
		if !info.IsDir() {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return fmt.Errorf("Failed to determine relative path: %v", err)
			}
			cust.Index[rel], err = getFInfo(path)
		}
		return err
	})
	if err != nil {
		return nil, err
	}

	fd, err := os.Open(vers_root + "mutables.list")
	switch {
	case err == nil:
		scanner := bufio.NewScanner(fd)
		for scanner.Scan() {
			path := strings.TrimSpace(scanner.Text())
			if len(path) == 0 {
				continue
			}
			cust.Mutables = append(cust.Mutables, path)
			if _, ok := cust.Index[path]; !ok {
				log.Printf("W: File \"%s\" from mutables.list isn't present in /files/", path)
			}
		}
		_ = fd.Close()

		if err = scanner.Err(); err != nil {
			return nil, fmt.Errorf("Reading mutables.list failed:", err)
		}

	case os.IsNotExist(err):

	default:
		return nil, fmt.Errorf("Reading mutables.list failed:", err)
	}
	return cust, nil
}

func checkAssets(id string, dl *AssetDownload) (err error) {
	log.Printf("Checking assets \"%s\"...\n", id)

	version := id
	key := version + ".json"
	path := storeRoot + "assets/indexes/" + version + ".json"
	if dl.SHA1 != "" {
		key = dl.SHA1
		path = storeRoot + "assets/indexes/" + dl.SHA1 + "/" + version + ".json"
	}

	if checked.indexes[key] {
		if verbose {
			log.Printf("Index \"%s\" already checked\n", key)
		}
		return nil
	}

	if err = os.MkdirAll(storeRoot+"assets/indexes/", os.ModeDir|0755); err != nil {
		return err
	}
	if err = os.MkdirAll(storeRoot+"assets/objects/", os.ModeDir|0755); err != nil {
		return err
	}

	list, err := parseIndex(path)
	switch {
	case err == nil:

	case os.IsNotExist(err):
		if dl.URL == "" {
			dl.URL = url.Indexes + version + ".json"
		}
		err = getFile(&dl.Download, path)
		if err != nil {
			return err
		}
		list, err = parseIndex(path)
		if err != nil {
			return err
		}

	default:
		return err
	}

	for name, a := range list.Data {
		if checked.assets[a.Hash] {
			if verbose {
				log.Printf("Already checked: \"%s\"(%s)\n", name, a.Hash)
			}
			continue
		}

		if len(a.Hash) != 40 || a.Size <= 0 {
			return fmt.Errorf("asset \"%s\"(%s) size or hash defined incorrect", name, a.Hash)
		}

		localPath := a.Hash[:2] + "/" + a.Hash

		err := checkHash(storeRoot+"assets/objects/"+localPath, a.Hash)
		switch {
		case err == nil:
			if verbose {
				log.Printf("Exist: \"%s\"(%s)\n", name, a.Hash)
			}
			checked.assets[a.Hash] = true
			continue

		case strings.HasPrefix(err.Error(), "Invalid hash"):
			return err

		case os.IsNotExist(err):

		default:
			log.Printf("%v. Regetting", err)
		}

		err = getFile(&Download{
			SHA1: a.Hash,
			Size: a.Size,
			URL:  url.Assets + localPath,
		}, storeRoot+"assets/objects/"+localPath)
		if err != nil {
			return err
		}

		checked.assets[a.Hash] = true
	}

	checked.indexes[key] = true
	return
}

func clean() {
	log.Println("Cleaning up...\n")
	indexesRoot := storeRoot + "assets/indexes/"
	dir, err := ioutil.ReadDir(indexesRoot)
	if err != nil {
		log.Fatal("Can't read assets indexes directory", err)
	}
	for _, fi := range dir {
		if fi.IsDir() || checked.indexes[fi.Name()] {
			continue
		}
		err = os.Remove(indexesRoot + fi.Name())
		if err != nil {
			log.Fatal("Cleanup failed:", err)
		}
		if verbose {
			log.Printf("Index \"%s\" deleted", fi.Name())
		}
	}

	libsRoot := storeRoot + "libraries/"
	_ = filepath.Walk(libsRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil && !os.IsNotExist(err) {
			log.Println("While walking over libraries:", err)
		}
		if info.IsDir() {
			rmEmptyDirs(path)
			return nil
		} else {
			log.Println(path)
			key := strings.TrimPrefix(strings.TrimSuffix(path, ".sha1"), libsRoot)
			_, ok := checked.libs[key]
			if !ok && key != overwriteFile {
				err = os.Remove(path)
				if err != nil && !os.IsNotExist(err) {
					log.Fatal("Fatal:", err)
				} else if verbose {
					log.Printf("In libs: \"%s\" deleted", info.Name())
				}
				rmEmptyDirs(filepath.Dir(path))
			}
		}
		return nil
	})

	_ = filepath.Walk(storeRoot+"assets/objects/", func(path string, info os.FileInfo, err error) error {
		if err != nil && !os.IsNotExist(err) {
			log.Println("While walking over libraries:", err)
		}
		if info.IsDir() {
			rmEmptyDirs(path)
			return nil
		} else if !checked.assets[filepath.Base(path)] {
			err = os.Remove(path)
			if err != nil && !os.IsNotExist(err) {
				log.Fatal(err)
			} else if verbose {
				log.Printf("In assets: \"%s\" deleted", info.Name())
			}
			rmEmptyDirs(filepath.Dir(path))
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

func getFile(dl *Download, destPath string) error {
	name := filepath.Base(destPath)

	var (
		expectedHash []byte
		err          error
	)
	if dl.SHA1 != "" {
		expectedHash, err = hex.DecodeString(dl.SHA1)
		if err != nil || len(expectedHash) != 20 {
			return fmt.Errorf("invalid hash \"%s\" provided for \"%s\"", dl.SHA1, name)
		}
	}

	log.Printf("Getting file \"%s\"...", filepath.Base(destPath))

	if err := os.MkdirAll(filepath.Dir(destPath), os.ModeDir|0755); err != nil {
		return err
	}

	start := time.Now()
	resp, err := http.Get(dl.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("loading \"%s\" failed with status \"%s\"", dl.URL, resp.Status)
	}

	log.Printf("%s (%s)", resp.Status, readableSize(float64(resp.ContentLength)))

	if resp.ContentLength != -1 && dl.Size != 0 && resp.ContentLength != dl.Size {
		return fmt.Errorf("size of file \"%s\" does not match expectations", name)
	}

	fd, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer fd.Close()

	sha := sha1.New()
	out := io.MultiWriter(fd, sha)

	size, err := io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	delta := time.Now().Sub(start) + 1
	log.Printf("Done in %v, %s/s", delta, readableSize(float64(size)*float64(time.Second)/float64(delta)))

	if dl.Size != 0 && dl.Size != size {
		return fmt.Errorf("size of file \"%s\" does not match expectations", name)
	}
	hash := sha.Sum(nil)
	if dl.SHA1 != "" && !bytes.Equal(hash, expectedHash) {
		return fmt.Errorf("hash of file \"%s\" does not match expectations", name)
	}

	dl.Size = size
	dl.SHA1 = hex.EncodeToString(hash)
	return nil
}

func readableSize(in float64) string {
	var suffix = []string{"b", "kB", "MB", "GB", "TB", "PB"}
	sit := 0
	for in > 1024 {
		in /= 1024
		sit++
	}
	if sit >= len(suffix) {
		return "over9000"
	}
	return fmt.Sprintf("%.2f %s", in, suffix[sit])
}

func checkHash(path, hash string) error {
	dhash, err := hex.DecodeString(hash)
	if err != nil || len(dhash) != 20 {
		return fmt.Errorf("invalid hash \"%s\" provided for \"%s\"", hash, filepath.Base(path))
	}
	fhash, err := fileHash(path)
	if err != nil {
		return err
	}
	if bytes.Equal(dhash, fhash) {
		return nil
	}
	return fmt.Errorf("hash sums mismatched for \"%s\":\ndefined:\t %s \ncalicated:\t %s",
		filepath.Base(path), hash, hex.EncodeToString(fhash))

}

func getFInfo(path string) (info FInfo, err error) {
	s, err := os.Stat(path)
	if err != nil {
		return
	}
	info.Size = s.Size()

	fhash, err := fileHash(path)
	if err != nil {
		return
	}

	info.Hash = hex.EncodeToString(fhash)
	return
}

func fileHash(path string) ([]byte, error) {
	fd, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	h := sha1.New()
	_, err = io.Copy(h, fd)
	return h.Sum(nil), err
}

func inSlice(val string, sli []string) bool {
	for _, t := range sli {
		if val == t {
			return true
		}
	}
	return false
}
