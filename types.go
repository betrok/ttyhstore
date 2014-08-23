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
	Arguments		string		`json:"minecraftArguments"`
	LVersion		int			`json:"minimumLauncherVersion"`
	Assets			string		`json:"assets"`
	CustomAssets	bool		`json:"customAssets"`
	Libs			[]*LibInfo	`json:"libraries"`
	MainClass		string		`json:"mainClass"`
}
func newVInfoFull() *VInfoFull {
	return &VInfoFull{Libs: make([]*LibInfo, 0)}
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

type AssetsList struct {
	Data map[string] Asset	`json:"objects"`
}
func newAssetsList() *AssetsList {
	return &AssetsList{make(map[string] Asset)}
}

type Asset struct {
	Hash	string	`json:"hash"`
	Size	int64	`json:"size"`
}

var help_msg = `Usage: %s [command] [options] [args]

Commads:

	check <prefix1>/<version1> [<prefix2>/<version2>] [...]
		Check whatever specified clients are consistent,
		if possible download missing files from official repos.
	
	collect
		Check all client versions,
		geneate new versions.json in all prefixes.
		
	clone <off_version1> [<off_version2>] [...]
		Clone clients from official repos in default prefix.
		
	help
		Show this message.
		
Options:
	
	-v
		Be more verbose.
		
	--root=<path>
		Overwrite storage root, default may be set by $TTYH_STORE env variable.
		
	--ignore=[<prefix1>/]<version1>[,[<prefix2>/]<version2>][...]
		Don't check specified versions while collect.
		If prefix not provided will search in default.
		
	--prefix=<prefix>
		Set for default clone. Predefined is "default".
		
	--last=<prefix1>/<type1>:<version1>[,<prefix2>/<type2>:<version2>][...]
		Overwrite latest versions in versions.json manualy.
		Default choise based on releaseTime in <version>.json
`

