# zipcop

Zipcop is a tool that monitors directory tree(s) for Zip/JAR archives, and adds/updates the files contained in those archives as specified in the `filesToReplace` map in `main.go`.

## Building

```
go get github.com/klustic/zipcop
go build github.com/klustic/zipcop
```

## Usage

Using the recursion feature will not only add watches for all subdirectories found at runtime, it will continue to add watches for new subdirectories as they are created:

```
zipcop -recurse <dir1> [dir2 [dir3 ...]]
```
