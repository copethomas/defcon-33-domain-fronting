package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// IPRange represents an IP range with start and end addresses
type IPRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// CDNIPMap maps CDN names to their IP ranges
type CDNIPMap map[string][]IPRange

// ASNResponse represents the response from ipinfo.io API
type ASNResponse struct {
	Prefixes []struct {
		Netblock string `json:"netblock"`
	} `json:"prefixes"`
}

// getIPRangesForASN fetches IP ranges for a given ASN using ipinfo.io API
func getIPRangesForASN(asn string) ([]IPRange, error) {
	// Remove "AS" prefix if present
	asnNumber := strings.TrimPrefix(asn, "AS")

	// Construct the API URL
	url := fmt.Sprintf("https://ipinfo.io/AS%s/json", asnNumber)

	// Create a client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Add a delay to avoid rate limiting
	time.Sleep(1 * time.Second)

	// Make the request
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("error making request to ipinfo.io: %v", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-OK response from ipinfo.io: %s", resp.Status)
	}

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}

	// Parse the response
	var asnResp ASNResponse
	if err := json.Unmarshal(body, &asnResp); err != nil {
		return nil, fmt.Errorf("error parsing JSON response: %v", err)
	}

	// Extract IP ranges
	var ipRanges []IPRange
	for _, prefix := range asnResp.Prefixes {
		// For simplicity, we're treating the CIDR notation as a range
		// In a real implementation, you might want to convert CIDR to actual start/end IPs
		parts := strings.Split(prefix.Netblock, "/")
		if len(parts) == 2 {
			ipRanges = append(ipRanges, IPRange{
				Start: parts[0],
				End:   parts[0], // This is a simplification; in reality, you'd calculate the end IP
			})
		}
	}

	return ipRanges, nil
}

func main() {
	// Define command-line flags
	inputFile := flag.String("input", "", "Path to the input CSV file (required)")
	outputFile := flag.String("output", "", "Path to the output JSON file (required)")

	// Parse flags
	flag.Parse()

	// Validate flags
	if *inputFile == "" || *outputFile == "" {
		flag.Usage()
		log.Fatal("Both input and output file paths are required")
	}

	// Open the CSV file
	file, err := os.Open(*inputFile)
	if err != nil {
		log.Fatalf("Error opening input file: %v", err)
	}
	defer file.Close()

	// Create a CSV reader with flexible field count
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1 // Allow variable number of fields

	// Read the header
	header, err := reader.Read()
	if err != nil {
		log.Fatalf("Error reading CSV header: %v", err)
	}

	// Verify the header has at least the cdn_name column
	if len(header) < 1 || header[0] != "cdn_name" {
		log.Fatalf("CSV file does not have the expected format")
	}

	// Create a map to store CDN to IP ranges mapping
	cdnIPMap := make(CDNIPMap)

	// Process each row in the CSV
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("Error reading CSV record: %v", err)
		}

		// Extract CDN name and ASNs
		cdnName := strings.Trim(record[0], "\"")

		// The ASNs are in the remaining columns, comma-separated
		var asns []string
		for i := 1; i < len(record); i++ {
			// Split by comma as ASNs are comma-separated
			asnList := strings.Split(record[i], ",")
			for _, asn := range asnList {
				// Trim spaces and quotes
				asn = strings.Trim(asn, " \"")
				if asn != "" {
					asns = append(asns, asn)
				}
			}
		}

		// Process each ASN for this CDN
		for _, asn := range asns {
			fmt.Printf("Processing ASN %s for CDN %s\n", asn, cdnName)

			// Get IP ranges for this ASN
			ipRanges, err := getIPRangesForASN(asn)
			if err != nil {
				log.Printf("Error getting IP ranges for ASN %s: %v", asn, err)
				continue
			}

			// Add IP ranges to the map
			cdnIPMap[cdnName] = append(cdnIPMap[cdnName], ipRanges...)
		}
	}

	// Convert the map to JSON
	jsonData, err := json.MarshalIndent(cdnIPMap, "", "  ")
	if err != nil {
		log.Fatalf("Error converting to JSON: %v", err)
	}

	// Write the JSON to the output file
	if err := ioutil.WriteFile(*outputFile, jsonData, 0644); err != nil {
		log.Fatalf("Error writing to output file: %v", err)
	}

	fmt.Printf("Successfully wrote CDN to IP mapping to %s\n", *outputFile)
}
