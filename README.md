### About
**ttyhstore** is a tool for managing unofficial minecraft update server.

It features:
*   `versions.json` file generation.
*   `libraries`, `assets` check and cleanup with ability to download missing files from official repository.
*   Custom files (e.g. mod files) are also supported.

### License
This software is licensed under MIT license. The full text of license and list your right you can find in `LICENSE` file.

### Repository filesystem structure
Data storage model mostly follows official update server model (see [wiki.vg](http://wiki.vg/Game_Files) for details).

All paths from now is relative to *storage root* directory.

Filesystem structure:
*   `prefixes.json`
    
    Contains list of prefixes collected from `<prefix>/prefix.json`.
    
    File contents:
    
        {
            "prefixes": {
                "<prefix>": {
                    "about": "<about>",
                    "type": "<type>"
                },
                [...]
            }
        }
    
    There:
    *   Field `about`
    
        Collected `about` field from `<prefix>/prefix.json`
        
    *   Field `type`
    
        Collected `type` field from `<prefix>/prefix.json`

*   `<prefix>/prefix.json`
    
    File contents:
    
        {
            "about": "<about>",
            "type": "<type>",
            "latest": {
                "<version_type_1>": "<version_name_1>",
                [...]
            }
        }
    
    There:
    *   Field `about`
    
        Contains short description about this prefix.
        
    *   Field `type`
    
        If this field contains `hidden` tag, this prefix will not be listed. Everything else in this field will mark this prefix as *public*, and it *will be* listed in `prefixes.json`.
        
    *   Field `latest`
    
        Contains object with following fields:
        
        *   `<client_type>` fields
        
            Name of the field is a description about version contained in value.
            You can add, for example `release` as `<client_type>` with `1.7.10` value, to tag this version as a `release`.
            This field can be exactly the same as `<prefix>/<version>/<version>.json` `type` field.
        
        `latest` field is optional. It overrides `latest` versions field in `<prefix>/versions/versions.json`.
        If `latest` field is ommited, *latest* of the client versions will be choosed by `releaseTime` field.
        And in `<prefix>/versions/versions.json` `latest` field will be replaced with this:
        
            "latest": {
                "<client_type>": "<latest_client_version>"
            }
        
        There:
        *   `<client_type>`
        
            Will be `type` field from `<prefix>/<version>/<version>.json`
            
        *   `<latest_client_version>`
        
            Will be client version, choosed by `releaseTime` field.
    
    If file `<prefix>/prefix.json` does not exist, defaults will be loaded instead.
    Defaults are `{ "about": "", "type": "public" }`.
    
*   `<prefix>/versions/versions.json`
    
    Similar to [versions.json](http://s3.amazonaws.com/Minecraft.Download/versions/versions.json) from official update server, but for current prefix.
    
*   `<prefix>/<version>/<version>.jar`
    
    Jar file of this build. Nuff said.

*   `<prefix>/<version>/<version>.json`
    
    May contain optional non-standard fields, used to check .jar files integrity:
    *   Field `jarHash`
    
        Contains sha1sum of .jar file as a value
        
    *   Field `jarSize`
    
        Contains file size as a value
    
    Example:
    
        "jarHash": "<sha1sum>",
        "jarSize": "<filesize>"
    
    Everything else is similar to official update server file structure.
    
*   `<prefix>/<version>/data.json`
    
    Generated automatically on cli checking.
    
    File contents:
    
        {
            "main": {
                "hash": "<sha1sum>",
                "size": "<size>"
            },
            "libs": {
                {...}
            },
            "files": {
                "mutables": [
                    "<filename>",
                    [...]
                ],
                index: {
                    {...}
                }
            }
        }
    There:
    *   Field `main`
    
        Contains sha1sum and file size of `<prefix>/<version>/<version>.jar`
        
        *   Field `hash`
        
            Contains sha1sum as a value.
            
        *   Field `size`
        
            Contains file size as a value.
    
    *   Field `libs`
    
        Contains index of libraries, in usual index format.
    
    *   Field `files`
    
        Contains information about custom files
        
        *   Field `mutables`
        
            Contains list of mutable files, filled from plain text file `mutables.list`
            
        *   Field `index`
        
            Contains index of custom files, in usual index format. Paths are related to `<prefix>/<version>/files/` directory.
    
    
*   `<prefix>/<version>/files/`
    
    Contains custom files, e.g. server.dat or mods.
    
*   `<prefix>/<version>/mutables.list`
    
    Plain text list of files, thats may be changed by user.
    
*   `libraries/`
    
    Similar to [libraries.minecraft.net](https://libraries.minecraft.net/).

*   `assets/indexes/`
    
    Contain asserts indexes(`<asserts version>.json`), similar to [indexes folder](https://s3.amazonaws.com/Minecraft.Download/indexes/) on official update server.
    
*   `assets/objects/<first 2 hex letters of hash>/<whole hash>`
    
    Assets files.

Libraries and assets are shared between all prefixes and versions.

### Usage

First of all you need to define `TTYH_STORE` env variable.
```bash
    export TTYH_STORE="/path/to/repository"
```
Value of this variable points to repository location. You can also use `--root` option, but it is less comfortable.

#### Minimal example
Clone specified versions from official repository to default prefix:
```bash
    ttyhstore clone 1.7.4 1.7.10
```

Check all clients and generate `versions.json` in all prefixes. `prefixes.json` will be also updated.
```bash
    ttyhstore collect
```

Done, now you have your own minecraft update server with official 1.7.4 and 1.7.10 versions. At least after you add storage root to web server.

#### Custom client

Create `<prefix>/<your version>/` directory, place there `<version>.json` and `<version>.jar` files.

For libraries, that aren't presented in official repo, copy `<lib name>.jar` and `<lib name>.jar.sha1` files to `libraries/` following minecraft path policy.

If your build need some specific files, place them in `<prefix>/<your version>/files/`. Index will be generated on `ttyhstore check`.

To make sure that everything is correct and to download missing asserts and libraries, run:
```bash
    ttyhstore check $prefix/$your_version
```

And then regenerate `versions.json`
```bash
    ttyhstore collect
```

#### Delete version or prefix

Just delete directory from repo and run `ttyhstore collect` to exclude it from all lists.

You may also remove all asserts and libraries, that aren't required by any client:
```bash
    ttyhstore cleanup
```

#### More

See `ttyhstore help`.