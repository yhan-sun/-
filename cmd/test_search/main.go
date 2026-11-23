package main

import (
	"fmt"
	"log"

	"hongguo/pkg/device"
	"hongguo/pkg/hgapi"
)

func main() {
	fmt.Println("==> Step 1: Registering anonymous device...")
	identity, err := device.RegisterDevice()
	if err != nil {
		log.Fatalf("Device registration failed: %v", err)
	}
	fmt.Printf("Device registered successfully!\n  DeviceID: %s\n  IID:      %s\n  CDID:     %s\n",
		identity.DeviceID, identity.IID, identity.CDID)

	keyword := "保洁"
	fmt.Printf("\n==> Step 2: Searching keyword: %q (page 1)...\n", keyword)
	dramas, err := hgapi.Search(keyword, 1, identity)
	if err != nil {
		log.Fatalf("Search failed: %v", err)
	}

	fmt.Printf("Search returned %d results:\n", len(dramas))
	for i, d := range dramas {
		fmt.Printf("  [%d] %s (SeriesID: %s, Episodes: %d)\n", i+1, d.SeriesTitle, d.SeriesID, d.EpisodeCount)
	}
	fmt.Println("\n==> P0 verification: Go native crypto & search API PASSED 100%!")
}
