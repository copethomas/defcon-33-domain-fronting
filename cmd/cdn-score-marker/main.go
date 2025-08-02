package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// TestData represents the JSON structure of each test object
type TestData struct {
	TestID          string   `json:"test_id"`
	TestType        string   `json:"test_type"`
	AttackHost      string   `json:"attack_host"`
	AttackURL       string   `json:"attack_url"`
	FrontDomain     string   `json:"front_domain"`
	ResponseHeaders []string `json:"response_headers"`
	OutputRes       string   `json:"output_res"`
	OutputDigest    string   `json:"output_digest"`
	TestResult      string   `json:"test_result"`
	HostIP          string   `json:"host_ip"`
	FrontDomainIP   string   `json:"front_domain_ip"`
	OriginalDigest  string   `json:"original_digest"`
	FhfdDigest      *string  `json:"fhfd_digest"`
}

// CDNData represents the CSV structure for CDN domain data
type CDNData struct {
	CDN       string
	DomainSLD string
	IPAddr    string
}

func main() {
	// Define command-line flags
	inputFile := flag.String("fronting_success_cases", "", "Path to the input fronting_success_cases JSON file (required)")
	csvFile := flag.String("domains_to_cdn", "", "Path to the domains_to_cdn file with CDN domain data (required)")
	flag.Parse()

	// Check if input file was provided
	if *inputFile == "" || *csvFile == "" {
		fmt.Println("Error: Both input JSON file and CSV file are required")
		fmt.Println("Usage: cdn-score-marker -input <json_filename> -csv <csv_filename>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Check if the CSV file exists
	if _, err := os.Stat(*csvFile); os.IsNotExist(err) {
		fmt.Printf("Error: CSV file '%s' does not exist\n", *csvFile)
		os.Exit(1)
	}

	// Check if the file exists
	if _, err := os.Stat(*inputFile); os.IsNotExist(err) {
		fmt.Printf("Error: File '%s' does not exist\n", *inputFile)
		os.Exit(1)
	}

	// Read the file
	fileData, err := os.ReadFile(*inputFile)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Parse the JSON data
	var testDataArray []TestData
	err = json.Unmarshal(fileData, &testDataArray)
	if err != nil {
		fmt.Printf("Error parsing JSON: %v\n", err)
		os.Exit(1)
	}

	// Read and parse the CSV file
	csvData, err := os.Open(*csvFile)
	if err != nil {
		fmt.Printf("Error opening CSV file: %v\n", err)
		os.Exit(1)
	}
	defer csvData.Close()

	// Create a new CSV reader
	reader := csv.NewReader(csvData)

	// Read the header line
	_, err = reader.Read()
	if err != nil {
		fmt.Printf("Error reading CSV header: %v\n", err)
		os.Exit(1)
	}

	// Read all CSV records
	var cdnDataArray []CDNData
	records, err := reader.ReadAll()
	if err != nil {
		fmt.Printf("Error reading CSV data: %v\n", err)
		os.Exit(1)
	}

	// Parse CSV records into CDNData structs
	for _, record := range records {
		if len(record) == 3 {
			cdnDataArray = append(cdnDataArray, CDNData{
				CDN:       record[0],
				DomainSLD: record[1],
				IPAddr:    record[2],
			})
		}
	}

	// Create a map for quick domain lookup
	domainToCDN := make(map[string]string)
	for _, data := range cdnDataArray {
		domainToCDN[data.DomainSLD] = data.CDN
	}

	// Create a counter for test types by CDN
	cdnTestTypeCounter := make(map[string]map[string]int)

	// Process each test
	fmt.Println("\nProcessing tests:")
	for _, test := range testDataArray {
		// Parse the front domain to get the host
		frontDomain := test.FrontDomain

		// If the domain starts with http:// or https://, parse it
		var host string
		if strings.HasPrefix(frontDomain, "http://") || strings.HasPrefix(frontDomain, "https://") {
			parsedURL, err := url.Parse(frontDomain)
			if err != nil {
				fmt.Printf("Error parsing URL %s: %v\n", frontDomain, err)
				continue
			}
			host = parsedURL.Host
		} else {
			host = frontDomain
		}

		// Remove port if present
		if colonIndex := strings.Index(host, ":"); colonIndex != -1 {
			host = host[:colonIndex]
		}

		// Check if the domain is in our CDN list
		cdn, found := domainToCDN[host]
		if !found {
			fmt.Printf("Error: Domain %s not found in CDN list. Aborting.\n", host)
			os.Exit(1)
		}

		// Increment the counter for this test type and CDN
		if _, exists := cdnTestTypeCounter[cdn]; !exists {
			cdnTestTypeCounter[cdn] = make(map[string]int)
		}
		cdnTestTypeCounter[cdn][test.TestType]++

		fmt.Printf("Test ID: %s, Front Domain: %s, CDN: %s, Test Type: %s\n",
			test.TestID, host, cdn, test.TestType)
	}

	dfcdn := make(map[string]int)
	for cdn, testTypes := range cdnTestTypeCounter {
		for _, count := range testTypes {
			if count > 10 {
				dfcdn[cdn] = count
			}
		}
	}
	fmt.Println("---")
	fmt.Println("CDNs supporting domain fronting:")
	fmt.Println("---")
	for cdn, count := range dfcdn {
		fmt.Printf("CDN: %s, Count: %d\n", cdn, count)
	}
}
