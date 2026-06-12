//go:build ignore

package main

import (
	"fmt"
	
	u "github.com/nanjj/slingshot/internal/usage"
)

func main() {
	usage := u.Usage{
		u.Name,
		u.File.List(1),
	}

	// 3 files
	args := []string{"testsite", "file1.org", "file2.org", "file3.org"}
	fmt.Printf("Args: %v\n", args)
	parsed := u.Parse(usage, args)
	fmt.Printf("Parsed count: %d\n", len(parsed))
	for i, p := range parsed {
		fmt.Printf("  parsed[%d]: String=%q Skipped=%v StringList=%v\n", i, p.String, p.Skipped, p.StringList)
	}
	fmt.Println("OK: 3 files accepted")

	// 1 file
	args2 := []string{"testsite", "single.org"}
	parsed2 := u.Parse(usage, args2)
	fmt.Printf("1 file: parsed[1].StringList=%v\n", parsed2[1].StringList)
	fmt.Println("OK: 1 file accepted")
	
	// 0 files
	args3 := []string{"testsite"}
	parsed3 := u.Parse(usage, args3)
	hasError := len(parsed3) < 2 || parsed3[1].Skipped || len(parsed3[1].StringList) == 0
	fmt.Printf("0 files: Skipped=%v StringList=%v -> error=%v\n", 
		len(parsed3) >= 2 && parsed3[1].Skipped,
		len(parsed3) >= 2 && parsed3[1].StringList,
		hasError,
	)
	if hasError {
		fmt.Println("OK: 0 files correctly rejected")
	}
}
