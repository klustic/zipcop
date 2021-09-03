# zipcop

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
