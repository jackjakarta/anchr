## Install

Just download from releases.

```bash
tar -xzf anchr-darwin-arm64.tar.gz   # or whichever platform matches
```

on macOS maybe you need to do this, but try without first:

```
xattr -d com.apple.quarantine "$INSTALL_PATH"
```

macOS config location:

```
/Library/Application Support/anchr
```

anchr binary location:

```
/usr/local/bin
```