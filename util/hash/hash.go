package main

import (
	"fmt"
	"os"
	"strconv"

	"golang.org/x/crypto/bcrypt"
)

func usage() {
	fmt.Printf(`bcrypt password hash generator
usage:
	%[1]s validate <hash> <password>
			checks the password matches the hash
	%[1]s generate <password>
			generates a hash based on the password [cost=%[2]d]
	%[1]s generate <cost> <password>
			generates a hash with specified cost [%[3]d..%[4]d]
`,
		os.Args[0],
		bcrypt.DefaultCost,
		bcrypt.MinCost,
		bcrypt.MaxCost)
	os.Exit(1)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}

	switch os.Args[1] {
	case "validate":
		if err := bcrypt.CompareHashAndPassword([]byte(os.Args[2]), []byte(os.Args[3])); err != nil {
			fmt.Println("bad: ", err)
			os.Exit(1)
		}
		fmt.Println("good: matches")
	case "generate":
		var hash []byte
		var cost int64
		if len(os.Args[2:]) > 1 {
			cost, _ = strconv.ParseInt(os.Args[2], 0, 0)
			hash = []byte(os.Args[3])
		} else {
			hash = []byte(os.Args[2])
		}
		hash, err := bcrypt.GenerateFromPassword(hash, int(cost))
		if err != nil {
			fmt.Println(err)
		}
		fmt.Printf("%s\n", hash)
	default:
		usage()
	}

}
