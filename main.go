package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"vs_export/sln"
)

func main() {
	path := flag.String("s", "", "sln or vcxproj file path")
	configuration := flag.String("c", "Debug|x64",
		"Configuration, [configuration|platform], default Debug|x64")
	flag.Parse()

	if *path == "" {
		usage()
		os.Exit(1)
	}

	solution, err := sln.NewSln(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cmdList, err := solution.CompileCommandsJson(*configuration)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	js, err := json.Marshal(cmdList)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%s\n", js[:])
	ioutil.WriteFile("compile_commands.json", js[:], 0644)
}

func usage() {
	var echo = `Usage: %s -s <path> -c <configuration>

Where:
            -s   path                        sln or vcxproj filename
            -c   configuration               project configuration,eg Debug|x64.
                                             default Debug|x64
	`
	echo = fmt.Sprintf(echo, filepath.Base(os.Args[0]))
	fmt.Println(echo)
}
