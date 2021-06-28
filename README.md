# bee-fs
FUSE filesystem for bee

Hybrid filesystem aiming to provide backup functionality to users. `bee-fs` can be used to mount folders on your local machine
which are backed up to a bee node from time to time. Users can configure snapshot intervals and also no. of snapshots to keep.

A postage batch can be configured per mount point.

Each snapshot is represented as a bee manifest. The manifest can be used to retrieve the entire filesystem information back
from the swarm network to any machine running `bee-fs`.

`bee-fs` uses [billziss-gh/cgofuse](https://github.com/billziss-gh/cgofuse). This was chosen as it is supported on all the
platforms (Windows included! Phew!)

The FUSE implementation is based on the in-memory filesystem implementation inside `cgofuse`. Additionally, the implementation
stores files using `bee-file (pkg/file)` instead of in-memory. This stores the writes in-memory till the file is `closed` or `synced` manually
and written in the format used by `bee`.

```

Used for FileSystem interface with swarm network

Usage:
   [command]

Available Commands:
  daemon      Start bee-fs daemon
  help        Help about any command
  mount       Mount bee-fs endpoint
  restore     bee-fs endpoint snapshot restore commands
  snapshot    bee-fs endpoint snapshot related commands

Flags:
  -h, --help   help for this command

Use " [command] --help" for more information about a command.
```

## Quickstart
```
1. Install [FUSE](http://github.com/libfuse/libfuse) for your OS

2. Install bee-fs
  bee-fs is go project. So you can clone it locally and build the `cmd` package
  
3. Run the daemon
  bee-fs daemon
  
4. Mount a directory
  bee-fs mount create <PATH TO DIRECTORY> --postage-batch <Batch ID> --snapshot-policy @hourly --use-badger
  
5. Apart from the snapshot schedule, you can manually trigger snapshots
  bee-fs snapshot create <PATH TO MOUNT>
  
6. See status of your snapshots or mounts
  bee-fs mount get /Users/aloknerurkar/beefs --snapshots
PATH                       ACTIVE  SNAPSHOT POLICY  KEEP COUNT  ENCRYPTION  NO OF SNAPSHOTS  PREVIOUS RUN                   STATUS      NEXT RUN
----                       ------  ---------------  ----------  ----------  ---------------  ------------                   ------      --------
/Users/aloknerurkar/beefs  true    @hourly          5           false       1                0001-01-01 00:00:00 +0000 UTC  successful  2021-06-28 19:00:00 +0530 IST



NAME             CREATED                               REFERENCE                                                         SYNCED
----             -------                               ---------                                                         ------
snap_1624885700  2021-06-28 18:38:20.583906 +0530 IST  7bd763b6ef47f4d0980180be8b9203cb105ef05b92c4046c533f454639aa886a  98%
