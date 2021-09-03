package main

import (
	"archive/zip"
	_ "embed"
	"flag"
	"io"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/klustic/fsnotify"
)

//go:embed resources/hello.txt
var a_test_txt []byte

//go:embed resources/hello.txt
var b_test_txt []byte

//go:embed resources/hello.txt
var c_test_txt []byte

// All files we want to replace, and their contents
var filesToReplace map[string][]byte

// ZIP files to ignore inotify events for
var shouldIgnore map[string]bool

func addFileToZip(zipPath string) error {
	var err error

	// Copy files to a local map
	localFiles := make(map[string][]byte)
	for k, v := range filesToReplace {
		localFiles[k] = v
	}

	// Open ZIP for reading
	zipReader, err := zip.OpenReader(zipPath)
	if err != nil {
		log.Printf("Encountered error opening ZIP for reading: %e\n", err)
		return err
	}
	defer zipReader.Close()

	// Open temporary ZIP for writing
	tempName := filepath.Join(filepath.Dir(zipPath), "."+filepath.Base(zipPath)+".swp")
	tempFile, err := os.OpenFile(tempName, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Printf("Error encountered opening temporary ZIP for writing: %e\n", err)
		return err
	}
	zipWriter := zip.NewWriter(tempFile)
	defer tempFile.Close()
	defer zipWriter.Close()

	// Iterate through ZIP file, updating/adding files of interested and copying the rest directly
	for _, f := range zipReader.File {
		if body, ok := localFiles[f.Name]; ok {
			// skip existing file and add new file
			w, _ := zipWriter.CreateHeader(&f.FileHeader)
			io.Writer.Write(w, body)
			delete(localFiles, f.Name)
			log.Printf("Updated -> %s:%s\n", zipPath, f.Name)
		} else {
			// add existing file
			zipWriter.Copy(f)
			// log.Printf("Copied  -> %s\n", f.Name)
		}
	}

	// Copy over any files that are left over
	for k, v := range localFiles {
		w, _ := zipWriter.Create(k)
		io.Writer.Write(w, v)
		log.Printf("Added  -> %s:%s\n", zipPath, k)
	}

	// Move temporary file over original; we want to make sure the inotify watch doesn't fire on this recursively!
	shouldIgnore[zipPath] = true
	os.Rename(tempName, zipPath)

	return nil
}

func main() {
	recursePtr := flag.Bool("recurse", false, "Recursively add watches to all subdirectories")
	flag.Parse()

	// Populate all files we want to replace
	filesToReplace = make(map[string][]byte)
	filesToReplace["a/test.txt"] = a_test_txt
	filesToReplace["b/test.txt"] = b_test_txt
	filesToReplace["c/test.txt"] = c_test_txt

	// inotify events for files in this map should be ignored
	shouldIgnore = make(map[string]bool)

	// Use SIGUSR1 to clear the map of files to be ignored
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGUSR1)
	go func() {
		for {
			<-sigs
			log.Println("Received SIGUSR1, clearing the ignored files cache")
			for k := range shouldIgnore {
				delete(shouldIgnore, k)
			}
		}
	}()

	// inotify watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	// monitor the inotify event queue
	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				info, err := os.Stat(event.Name)
				if err != nil {
					continue
				}

				// Modify JAR files
				if event.Op&fsnotify.CloseWrite == fsnotify.CloseWrite {

					// Check if filename is regular file and ends in .zip
					if info.Mode().IsRegular() && (filepath.Ext(event.Name) == ".zip" || filepath.Ext(event.Name) == ".jar") {
						zipPath, _ := filepath.Abs(event.Name)
						log.Printf("A ZIP/Jar file was written here: %s\n", zipPath)

						// Check if this file should be ignored
						if _, ok := shouldIgnore[zipPath]; ok {
							log.Printf("This file has already been dorked! Send me SIGUSR1 to clear my cache if you want to hit it again.")
							continue
						}

						// Write files to .zip
						go addFileToZip(zipPath)
					}
				}

				// Add watches to new directories
				// NOTE: recursive watches encounter a race condition, e.g. mkdir -p may win and cause a child directory to go unwatched
				if event.Op&fsnotify.Create == fsnotify.Create {
					if info.IsDir() && *recursePtr {
						if err = watcher.Add(event.Name); err != nil {
							log.Fatal(err)
						}
						log.Printf("Added a watch: %s\n", event.Name)
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("error:", err)
			}
		}
	}()

	// Add watches for all directories specified on the commandline
	if *recursePtr {
		log.Println("** Recursion is enabled! Adding watches for specified directories and all subdirectories")
	}

	watchedDirs := make(map[string]bool)

	// Gather all watchable directories
	for _, basePath := range flag.Args() {
		basePath, _ = filepath.Abs(basePath)

		s, err := os.Stat(basePath)
		if err != nil || !s.IsDir() {
			continue
		}

		if *recursePtr {
			// Do recursion
			filepath.WalkDir(basePath, func(b string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					watchedDirs[b] = true
				}
				return nil
			})

		} else {
			// Do simple watch
			watchedDirs[basePath] = true
		}
	}

	// Add watches for all valid directories
	for k := range watchedDirs {
		err = watcher.Add(k)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Added a watch: %s\n", k)
	}

	// Exit if we're not watching at least one directory
	if len(watchedDirs) == 0 {
		log.Fatalln("No watchable directories were specified, bailing")
	}
	<-done
}
