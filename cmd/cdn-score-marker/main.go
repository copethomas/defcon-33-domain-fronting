package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"strings"
)

const (
	DomainFrontSuccess = "DomainFrontSuccess"
	DomainFrontFailed  = "DomainFrontFailed"
)

// TestData represents the structure of each JSON object in the input file
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
}

// CdnDomainData represents the structure of each row in the CSV file
type CdnDomainData struct {
	Cdn       string
	DomainSld string
	IpAddr    string
}

func main() {
	// Define command-line flags
	inputFile := flag.String("fronting_test_details", "", "Path to the input fronting_test_details JSON file (required)")
	csvFile := flag.String("domains_csv", "", "Path to the CSV file with CDN domain data (required)")
	resultsFile := flag.String("results", "", "Path to the output CSV file with results (required)")
	flag.Parse()

	// Check if input file was provided
	if *inputFile == "" {
		fmt.Println("Error: Input file is required")
		fmt.Println("Usage: cdn-score-marker -fronting_test_details <filename> -domains_csv <csvfilename> -results <outputfilename>")
		os.Exit(1)
	}

	// Check if CSV file was provided
	if *csvFile == "" {
		fmt.Println("Error: CSV file is required")
		fmt.Println("Usage: cdn-score-marker -fronting_test_details <filename> -domains_csv <csvfilename> -results <outputfilename>")
		os.Exit(1)
	}

	// Check if results file was provided
	if *resultsFile == "" {
		fmt.Println("Error: Results file is required")
		fmt.Println("Usage: cdn-score-marker -fronting_test_details <filename> -domains_csv <csvfilename> -results <outputfilename>")
		os.Exit(1)
	}

	// Check if the input file exists
	if _, err := os.Stat(*inputFile); os.IsNotExist(err) {
		fmt.Printf("Error: File '%s' does not exist\n", *inputFile)
		os.Exit(1)
	}

	// Check if the CSV file exists
	if _, err := os.Stat(*csvFile); os.IsNotExist(err) {
		fmt.Printf("Error: CSV file '%s' does not exist\n", *csvFile)
		os.Exit(1)
	}

	// Read the file
	data, err := ioutil.ReadFile(*inputFile)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Parse the JSON data
	var testDataArray []TestData
	err = json.Unmarshal(data, &testDataArray)
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

	// Read all CSV records
	records, err := reader.ReadAll()
	if err != nil {
		fmt.Printf("Error reading CSV data: %v\n", err)
		os.Exit(1)
	}

	// Parse CSV records into CdnDomainData structs
	var cdnDomains []CdnDomainData
	// Skip the header row (first row)
	for i, record := range records {
		if i == 0 {
			// Skip header row
			continue
		}
		if len(record) == 3 {
			cdnDomains = append(cdnDomains, CdnDomainData{
				Cdn:       record[0],
				DomainSld: record[1],
				IpAddr:    record[2],
			})
		}
	}

	// Create a map for quick domain lookup
	domainMap := make(map[string]CdnDomainData)
	for _, domain := range cdnDomains {
		domainMap[domain.DomainSld] = domain
	}

	// Create a map to count test types by CDN
	cdnTestTypeCounts := make(map[string]map[string]int)

	// Process each test
	fmt.Println("Processing tests:")
	for _, testData := range testDataArray {
		// Parse the front domain to extract the domain
		frontDomain := testData.FrontDomain

		// Add scheme if missing to ensure proper URL parsing
		if !strings.HasPrefix(frontDomain, "http://") && !strings.HasPrefix(frontDomain, "https://") {
			frontDomain = "https://" + frontDomain
		}

		frontDomainURL, err := url.Parse(frontDomain)
		if err != nil {
			panic(fmt.Sprintf("Error parsing URL %s: %v\n", testData.FrontDomain, err))
		}

		// Extract the domain from the URL
		host := frontDomainURL.Host

		// Remove port if present
		if colonIndex := strings.Index(host, ":"); colonIndex != -1 {
			host = host[:colonIndex]
		}

		// Check if the domain is in our list
		cdnDomain, exists := domainMap[host]
		if !exists {
			panic(fmt.Sprintf("Error: Domain %s (from %s) not found in CDN domains list. Aborting.\n",
				host, testData.FrontDomain))
		}

		// Update the counter for this test type and CDN
		if _, exists := cdnTestTypeCounts[cdnDomain.Cdn]; !exists {
			cdnTestTypeCounts[cdnDomain.Cdn] = make(map[string]int)
		}
		cdnTestTypeCounts[cdnDomain.Cdn][testData.TestType]++

		if testData.TestType == "AHFD" { //This is the true "Domain Fronting" test case we care about
			switch testData.TestResult {
			case "Success":
				cdnTestTypeCounts[cdnDomain.Cdn][DomainFrontSuccess]++
				fmt.Printf("DomainFrontSuccess!!! = Matched test ID %s: front_domain=%s, domain_sld=%s, cdn=%s, test_type=%s, test_result=%s\n",
					testData.TestID, testData.FrontDomain, host, cdnDomain.Cdn, testData.TestType, testData.TestResult)
			case "Failed":
				cdnTestTypeCounts[cdnDomain.Cdn][DomainFrontFailed]++
			default:
				panic(fmt.Sprintf("Error: Invalid test result %s for test ID %s\n", testData.TestResult, testData.TestID))
			}
		}

		fmt.Printf("Matched test ID %s: front_domain=%s, domain_sld=%s, cdn=%s, test_type=%s\n",
			testData.TestID, testData.FrontDomain, host, cdnDomain.Cdn, testData.TestType)
	}

	// Print the test type counts by CDN
	fmt.Println("\nTest Type Counts by CDN:")
	totalTests := 0
	totalCdns := len(cdnTestTypeCounts)

	for cdn, testTypes := range cdnTestTypeCounts {
		cdnTotal := 0
		fmt.Printf("CDN: %s\n", cdn)

		for testType, count := range testTypes {
			fmt.Printf("  %s: %d\n", testType, count)
			cdnTotal += count
		}

		fmt.Printf("  Total for %s: %d\n\n", cdn, cdnTotal)
		totalTests += cdnTotal
	}

	fmt.Printf("Summary:\n")
	fmt.Printf("Total CDNs processed: %d\n", totalCdns)
	fmt.Printf("Total tests processed: %d\n", totalTests)

	// Write results to CSV file
	file, err := os.Create(*resultsFile)
	if err != nil {
		fmt.Printf("Error creating results file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"CDN", "DomainFrontSuccess", "DomainFrontFailed", "Total"}
	if err := writer.Write(header); err != nil {
		fmt.Printf("Error writing CSV header: %v\n", err)
		os.Exit(1)
	}

	// Write data for each CDN
	for cdn, testTypes := range cdnTestTypeCounts {
		successCount := testTypes[DomainFrontSuccess]
		failedCount := testTypes[DomainFrontFailed]
		total := successCount + failedCount

		record := []string{
			cdn,
			fmt.Sprintf("%d", successCount),
			fmt.Sprintf("%d", failedCount),
			fmt.Sprintf("%d", total),
		}

		if err := writer.Write(record); err != nil {
			fmt.Printf("Error writing CSV record: %v\n", err)
			os.Exit(1)
		}
	}

	// Ensure the writer flushes all buffered data to the underlying writer
	writer.Flush()

	// Check for any errors during flush
	if err := writer.Error(); err != nil {
		fmt.Printf("Error flushing CSV data: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nResults successfully saved to file: %s\n", *resultsFile)
}
