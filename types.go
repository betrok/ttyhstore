package main

import (
	"time"
)

type VInfoMin struct {
	Id		string		`json:"id"`
	Time	time.Time	`json:"time"`
	Release	time.Time	`json:"releaseTime"`
	Type	string		`json:"type"`
}

type Versions struct {
	Latest		map[string] string	`json:"latest"`
	latestTime	map[string] time.Time	//not in json
	Versions	[]*VInfoMin			`json:"versions"`
}

func newVersions() *Versions {
	return &Versions{
		make(map[string] string),
		make(map[string] time.Time),
		make([]*VInfoMin, 0, 10),
	}
}

type VersSlice []*VInfoMin
func (p VersSlice) Len() int           { return len(p) }
func (p VersSlice) Less(i, j int) bool { return p[i].Time.After(p[j].Time) }
func (p VersSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

type VInfoFull struct {
	VInfoMin
	JarHash			string		`json:"jarHash"`
	JarSize			int64		`json:"jarSize"`
	Arguments		string		`json:"minecraftArguments"`
	LVersion		int			`json:"minimumLauncherVersion"`
	Assets			string		`json:"assets"`
	Libs			[]*LibInfo	`json:"libraries"`
	MainClass		string		`json:"mainClass"`
}
func newVInfoFull() *VInfoFull {
	return &VInfoFull{ Libs: make([]*LibInfo, 0, 10) }
}

type LibInfo struct {
	Name	string				`json:"name"`
	Natives	map[string] string	`json:"natives"`
	Rules	[]Rule				`json:"rules"`
}

type Rule struct {
	Action	string				`json:"action"`
	Os		map[string]string	`json:"os"`
}

type ObjectList struct {
	Data FIndex	`json:"objects"`
}
func newObjectList() *ObjectList {
	return &ObjectList{ make(FIndex) }
}

type FInfo struct {
	Hash	string	`json:"hash"`
	Size	int64	`json:"size"`
}
type FIndex map[string]FInfo

type Customs struct {
	Mutables	[]string	`json:"mutables"`
	Index		FIndex		`json:"index"`
}
func newCustoms() *Customs {
	return &Customs{ make([]string, 0, 10), make(FIndex) }
}

type FilesInfo struct {
	Main	FInfo		`json:"main"`
	Libs	FIndex		`json:"libs"`
	Files	*Customs	`json:"files"`
}
func newFilesInfo() *FilesInfo {
	return &FilesInfo {
		Libs: make(FIndex),
		Files: newCustoms(),
	}
}

type PrefixInfo struct {
	About	string	`json:"about"`
	Type	string	`json:"type"`
}
type PrefixInfoExt struct {
	PrefixInfo
	Latest	map[string]string	`json:"latest"`
}

type PrefixList struct {
	Prefixes map[string]PrefixInfo	`json:"prefixes"`
}
func newPrefixList() *PrefixList {
	return &PrefixList{ make(map[string]PrefixInfo) }
}

var help_msg = `Usage: %s [options] [command] [args]

Commads:
	
	check [<prefix1>/]<version1> [[<prefix2>/]<version2>] [...]
		Check whatever specified clients are consistent,
		if possible download missing files from official repos.
		If prefix not provided will search in default.
	
	collect
		Check all client versions,
		geneate new versions.json in all prefixes.
		
	clone <off_version1> [<off_version2>] [...]
		Clone clients from official repos to default prefix.
		
	help
		Show this message.
		
	cleanup
		Alias to "collect --cleanup"
		
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
		Default chaise based on releaseTime in <version>.json.
		
	--cleanup
		After collect delete all libraries and assets,
		that not required by any client.
		Cleanup will aborting if any of clis is inconsistent.
`

