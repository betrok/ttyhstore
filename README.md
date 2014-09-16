### About

**ttyhstore** is a tool for menage unofficial minecraft update server.

It features versions.json generate and libraries/assets check/cleanup with ability to download missing from official repository. Custom files(e.g. for mods) are also supported.


### Repository file structure

Data store model mostly follows official(see [wiki.vg](http://wiki.vg/Game_Files) for details).

Relative to storage root:
*   **/prefixes.json**
    
    Contains list of prefixes generated from **/&lt;prefix>/prefix.json**. If prefix have type *"hidden"*, it will not append hire.
    ```
    {
        "prefixes": {
            "<prefix>": {
                "about": "<about>",
                "type": "<type>"
            },
            [...]
        }
    }
    ```

*   **/&lt;prefix>/prefix.json**
    ```
    {
        "about": "<about>",
        "type": "<type>",
        "latest": {
            "<vers_type1>": "<vers_name1>",
            [...]
        }
    }
    ```
    
    If this file is not presented defaults are `{"about" = "", "type" = "public"}`.
    
    Optional *"latest"* files overwrite latest versions in versions.json manually. Default chaise based on releaseTime in /&lt;version>.json 
    
*   **/&lt;prefix>/versions/versions.json**

   Simular to http://s3.amazonaws.com/Minecraft.Download/versions/versions.json for current prefix.
   
*   **/&lt;prefix>/&lt;version>/&lt;version>.jar**

*   **/&lt;prefix>/&lt;version>/&lt;version>.json**
    
    May contains optional non-standard fields, used for check .jar file:
    - `"jarHash": "<sha1 of <version>.jar>"`
    - `"jarSize": <size of <version>.jar>`
    
*   **/&lt;prefix>/&lt;version>/data.json**

    ```
    {
        "main": {
            "Hash": "<sha1 of <version>.jar>",
            "Size": <size of <version>.jar>
        },
        "objects": {
            [usual index for libraries, required by client. Any os and arch are included.]
        },
        "files": {
			"mutables": [
				array from mutables.list(see below)
			],
			index: {
				[usual index of files, located in /files/]
			}
        }
    }
    ```
    
    Generated on cli checking.
    
*   **/files/**

    Contains custom files, e.g. setvers.dat or mods.
    
*   **/mutables.list**

    Plain text list of files, thats may be changed by user.
    
*   **/libraries/**

    Similar to https://libraries.minecraft.net/.

*   **/assets/indexes/**

    Contain asserts indexes(**&lt;asserts version>.json**), similar to https://s3.amazonaws.com/Minecraft.Download/indexes/.
    
*   **/assets/objects/&lt;first 2 hex letters of hash>/&lt;whole hash>**

    Assets files.
    
    
Libraries and assets are shared between all prefixes and versions.

### Usage

First of all you need set **TTYH_STORE** env variable. It's define where will located storage root. You may also use *--root* option, but it's less comfortable.

#### Minimal example
Clone passed versions from official repository to default prefix:
```
ttyhstore clone 1.7.4 1.7.10
```
Check all clients and generate **versions.json** in all prefixes. **prefixes.json** will be also updated.
```
ttyhstore collect
```
Done, now you have your own minecraft update server with official 1.7.4 and 1.7.10 versions. At least after you will append storage root to web server.

#### Custom client

Create **/&lt;prefix>/&lt;your version>/** directory, place there **&lt;version>.json** and **&lt;version>.jar** files.

For libraries, that aren't presented in official repo, place **&lt;lib name>.jar** and **&lt;lib name>.jar.sha1** hash file to **/libraries/** follows minecraft path policy.

If your build need some specific files, place them in **/&lt;prefix>/&lt;your version>**, files in **/&lt;prefix>/&lt;your version>/files/**. Index will be generated on cli check.

To make sure that everything is correct and download missing asserts and libraries, run
```
ttyhstore check <prefix>/<your version>
```
Then regenerate **versions.json**
```
ttyhstore collect
```

#### Delete version/prefix

Just delete directory with it and run `ttyhstore collect` for exclude it from all lists.

You may also remove all asserts and libraries, that aren't required by any client
```
ttyhstore cleanup
```

#### More

See `ttyhstore help`.