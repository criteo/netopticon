package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/pprof"
	"strings"
	"time"
)

import (
	"github.com/criteo/netopticon/snmpmagic"
)

import (
	"github.com/soniah/gosnmp"
)

var (
	outputPath     string
	snmpIP         string
	snmpHostFile   string
	snmpCommunity  string
	concurrency    int
	cpuProfilePath string
)

func init() {
	flag.StringVar(
		&outputPath, "out", "netopticon-_TS_.json",
		"Output file path ('_TS_' will be replaced with current timestamp)",
	)
	flag.StringVar(
		&snmpIP, "ip", "",
		"Adress of host to query",
	)
	flag.StringVar(
		&snmpHostFile, "hosts", "",
		"Path to list of hosts to query",
	)
	flag.StringVar(
		&snmpCommunity, "community", "public",
		"SNMP community to use for query",
	)
	flag.IntVar(
		&concurrency, "concurrency", 8,
		"Concurrency level (maximum number of hosts to contact at a given time)",
	)
	flag.StringVar(
		&cpuProfilePath, "cpuprofile", "",
		"Write CPU profile to path",
	)
}

func main() {
	// Take time at run start, absolute value rounded to closest 5 minutes
	// Mon Jan 2 15:04:05 -0700 MST 2006
	timestampStr := time.Now().Round(5 * time.Minute).Format("2006-01-02-1504")

	flag.Parse()
	if snmpIP == "" && snmpHostFile == "" {
		fmt.Println("error: please provide a host IP or a host list file.")
		fmt.Println()
		flag.Usage()
		os.Exit(1)
	}

	if cpuProfilePath != "" {
		f, err := os.Create(cpuProfilePath)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	// Load hosts list from argument and possible file
	hosts, err := loadHostList()
	if err != nil {
		log.Fatal("could not load host list: ", err)
	}

	// Check we can create and write to output file
	outputPath = strings.Replace(outputPath, "_TS_", timestampStr, -1)
	fout, err := os.Create(outputPath)
	if err != nil {
		log.Fatal("could not create output file: ", err)
	}
	defer fout.Close()

	// Use buffered channels to reduce blocking
	work := make(chan string, concurrency)
	results := make(chan *DeviceData, concurrency)

	// Spawn requested quantity of workers
	for i := 0; i < concurrency; i++ {
		// Dumb worker grabs tasks from a channel and outputs results in another
		go func() {
			for host := range work {
				results <- fetch(host, snmpCommunity)
			}
		}()
	}

	// Continue looping as long as one of there are tasks that either:
	// - have to be distributed
	// - are waiting to be picked up
	// - are being processed (in-flight)
	// - have results waiting to be picked up
	output := make(map[string]*DeviceData)
	currTask := 0
	inFlight := 0
	for currTask < len(hosts) || len(work) > 0 || inFlight > 0 || len(results) > 0 {
		select {
		case unit := <-results:
			output[unit.Host] = unit
			inFlight -= 1
		default:
			time.Sleep(50 * time.Millisecond)
		}

		// Send more work as we make progress
		if currTask < len(hosts) && len(work) < cap(work) {
			work <- hosts[currTask]
			currTask += 1
			inFlight += 1
		}
	}
	close(work)

	// Serialize data to output file
	jsonEncoder := json.NewEncoder(fout)
	if err := jsonEncoder.Encode(output); err != nil {
		log.Fatal(err)
	}

	fout.Sync()
	fout.Close()
}

// Builds a host list using both the host and hostfile CLI options.
// Assumes hostfile contains one host per line.
func loadHostList() ([]string, error) {
	var hosts []string

	if snmpIP != "" {
		hosts = append(hosts, snmpIP)
	}

	if snmpHostFile != "" {
		fin, err := os.Open(snmpHostFile)
		if err != nil {
			return nil, err
		}
		defer fin.Close()

		lines := bufio.NewScanner(fin)
		for lines.Scan() {
			hosts = append(hosts, lines.Text())
		}
		if err := lines.Err(); err != nil {
			return nil, err
		}
	}

	return hosts, nil
}

// Fetches and parses device data from a given host. May encounter errors which
// will be stored in the DeviceData.
func fetch(host string, snmpCommunity string) *DeviceData {
	// Copy default client settings to avoid data races between concurrent workers
	client := *gosnmp.Default
	client.Target = host
	client.Community = snmpCommunity
	client.Version = gosnmp.Version2c

	var MIBData OpticsMIB
	magic, err := snmpmagic.NewSNMPMagic(&MIBData)

	if err != nil {
		return NewDeviceDataError(host, err.Error())
	}

	if err := magic.Query(&client); err != nil {
		return NewDeviceDataError(host, err.Error())
	}

	return NewDeviceData(host, &MIBData)
}
