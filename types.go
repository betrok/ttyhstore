package main

import (
	"time"
)

type VersionManifest struct {
	Latest   map[string]string `json:"latest"`
	Versions []VInfoMin        `json:"versions"`
}

type VInfoMin struct {
	Id      string    `json:"id"`
	Time    time.Time `json:"time"`
	Release time.Time `json:"releaseTime"`
	Type    string    `json:"type"`
	// url for <version>.json
	URL string `json:"url"`
}

type Download struct {
	URL  string `json:"url"`
	Size int64  `json:"size"`
	SHA1 string `json:"sha1"`
}

func (dl Download) Match(info FInfo) bool {
	if dl.Size != 0 && info.Size != dl.Size {
		return false
	}
	if dl.SHA1 != "" && info.Hash != dl.SHA1 {
		return false
	}
	return true
}

func (dl Download) ToFInfo() FInfo {
	return FInfo{
		Hash: dl.SHA1,
		Size: dl.Size,
	}
}

type Prefix struct {
	Latest     map[string]string    `json:"latest"`
	latestTime map[string]time.Time //not in json
	Versions   []*VInfoMin          `json:"versions"`
}

func NewPrefix() *Prefix {
	return &Prefix{
		Latest:     make(map[string]string),
		latestTime: make(map[string]time.Time),
		Versions:   make([]*VInfoMin, 0, 10),
	}
}

type VersionSlice []*VInfoMin

func (p VersionSlice) Len() int           { return len(p) }
func (p VersionSlice) Less(i, j int) bool { return p[i].Time.After(p[j].Time) }
func (p VersionSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

type VInfoFull struct {
	VInfoMin
	LVersion   int           `json:"minimumLauncherVersion"`
	Assets     string        `json:"assets"`
	AssetIndex AssetDownload `json:"assetIndex"`
	Downloads  struct {
		Client Download `json:"client"`
		Server Download `json:"server"`
	}
	Libs         []LibInfo `json:"libraries"`
	MainClass    string    `json:"mainClass"`
	OldArguments string    `json:"minecraftArguments"`
	// irregular structure, store does not use it anyways
	Arguments interface{} `json:"arguments"`
	// nobody cares
	Loggging interface{} `json:"loggging"`
}

type AssetDownload struct {
	Download
	ID        string `json:"id"`
	TotalSize int64  `json:"totalSize"`
}

type LibDownload struct {
	Download
	Path string `json:"path"`
}

type LibInfo struct {
	Name      string `json:"name"`
	Url       string `json:"url"`
	Downloads *struct {
		Artifact    LibDownload            `json:"artifact"`
		Classifiers map[string]LibDownload `json:"classifiers"`
	} `json:"downloads"`
	Extract struct {
		Exclude []string `json:"exclude"`
	} `json:"extract"`
	Natives map[string]string `json:"natives"`
	Rules   []Rule            `json:"rules"`
}

type OsRule struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Arch    string `json:"arch"`
}

type Rule struct {
	Action   string                 `json:"action"`
	Os       OsRule                 `json:"os"`
	Features map[string]interface{} `json:"features"`
}

type ObjectList struct {
	Data FIndex `json:"objects"`
}

func NewObjectList() *ObjectList {
	return &ObjectList{make(FIndex)}
}

type FInfo struct {
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}
type FIndex map[string]FInfo

type Customs struct {
	Mutables []string `json:"mutables"`
	Index    FIndex   `json:"index"`
}

func NewCustoms() *Customs {
	return &Customs{make([]string, 0, 10), make(FIndex)}
}

type FilesInfo struct {
	Main  FInfo    `json:"main"`
	Libs  FIndex   `json:"libs"`
	Files *Customs `json:"files"`
}

func NewFilesInfo() *FilesInfo {
	return &FilesInfo{
		Libs:  make(FIndex),
		Files: NewCustoms(),
	}
}

type PrefixInfo struct {
	About string `json:"about"`
	Type  string `json:"type"`
}
type PrefixInfoExt struct {
	PrefixInfo
	Latest map[string]string `json:"latest"`
}

type PrefixList struct {
	Prefixes map[string]PrefixInfo `json:"prefixes"`
}

func NewPrefixList() *PrefixList {
	return &PrefixList{make(map[string]PrefixInfo)}
}

var helpMessage = `Usage: %s [options] [command] [args]

Commands:
	
	check [<prefix1>/]<version1> [[<prefix2>/]<version2>] [...]
		Check whatever specified clients are consistent,
		if possible download missing files from official repos.
		If prefix isn't provided will search in default.
	
	collect
		Check all client versions,
		geneate new versions.json in all prefixes.
		
	clone <off_version1> [<off_version2>] [...]
		Clone clients from official repos to default prefix.
		
	help
		Show this message.
		
	cleanup
		Alias to "--cleanup collect"
		
Options:
	
	-v
		Be more verbose.
		
	--root=<path>
		Overwrite storage root, default may be set by $TTYH_STORE env variable.
		
	--ignore=<prefix1>/<version1>[,<prefix2>/<version2>][...]
		Don't check specified versions while collect.
		
	--prefix=<prefix>
		Set default prefix for clone or check. Predefined is "default".
		
	--last=<prefix1>/<type1>:<version1>[,<prefix2>/<type2>:<version2>][...]
		Overwrite latest versions in versions.json manually.
		Default choice based on releaseTime in <version>.json.
		
	--cleanup
		After collect delete all libraries and assets,
		that aren't required by any client.
		Cleanup will be abort if any of clients is inconsistent.
	
	--replace
		Replace existing libraries if they do not match expectations.
`
