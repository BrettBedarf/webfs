# WebFS

A FUSE-based filesystem that mounts web resources (HTTP URLs, S3, etc.) as local files.

## Usage

```bash
go run ./cmd/ -nodes examples/pub_sources.json <mountpoint>
```

## Build

```bash
go build -o webfs ./cmd/
```

## DONE

- [x] Python POC
- [x] [Migrate & Iterate Python POC to Go MVP](https://github.com/BrettBedarf/webfs/pull/1)

## IN PROGRESS

- [ ] Docs
- [ ] e2e Tests
## TODO


- [ ] Socket API
- [ ] "Magic File" add i.e. `echo "https://example.com/file.txt" > /webfs/file.txt.wfs`
- [ ] Additional Adapters
  - [ ] S3
  - [ ] FTP
  - [ ] SFTP
  - [ ] Google Drive
  - [ ] Dropbox
  - [ ] OneDrive
  - [ ] GCS
  - [ ] Azure Blob Storage
  - [ ] IPFS
- Lua? Adapters extension point
- [ ] Rename
  - FTW (FUSE the Web), AnyFS, WebFuse, WebFusion, WFS, AnyFuse, AnyFusion, Net{Fuse,FS,...},
