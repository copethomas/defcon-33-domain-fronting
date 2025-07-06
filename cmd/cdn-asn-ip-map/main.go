package main

import (
	"bufio"
	"compress/gzip"
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
)

// IPRange represents an IP range with start and end addresses
type IPRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// CDNIPMap maps CDN names to their IP ranges
type CDNIPMap map[string][]IPRange

// ASNEntry represents an entry in the ip2asn-v4.tsv file
type ASNEntry struct {
	RangeStart    string
	RangeEnd      string
	ASNumber      string
	CountryCode   string
	ASDescription string
}

// Global variable to store ASN entries loaded from the TSV file
var asnEntries []ASNEntry

// checkAndDownloadTSV checks if ip2asn-v4.tsv exists, and if not, downloads and extracts it
func checkAndDownloadTSV() error {
	// Check if the file already exists
	if _, err := os.Stat("ip2asn-v4.tsv"); err == nil {
		fmt.Println("ip2asn-v4.tsv already exists, using existing file")
		return nil
	}

	fmt.Println("ip2asn-v4.tsv not found, downloading from iptoasn.com...")

	// Download the gzipped file
	resp, err := http.Get("https://iptoasn.com/data/ip2asn-v4.tsv.gz")
	if err != nil {
		return fmt.Errorf("error downloading ip2asn-v4.tsv.gz: %v", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received non-OK response from iptoasn.com: %s", resp.Status)
	}

	// Create a temporary file to store the gzipped content
	gzFile, err := os.Create("ip2asn-v4.tsv.gz")
	if err != nil {
		return fmt.Errorf("error creating temporary file: %v", err)
	}
	defer gzFile.Close()

	// Copy the response body to the temporary file
	_, err = io.Copy(gzFile, resp.Body)
	if err != nil {
		return fmt.Errorf("error saving downloaded file: %v", err)
	}

	// Close the file to ensure all data is written
	gzFile.Close()

	// Open the gzipped file for reading
	gzFile, err = os.Open("ip2asn-v4.tsv.gz")
	if err != nil {
		return fmt.Errorf("error opening gzipped file: %v", err)
	}
	defer gzFile.Close()

	// Create a gzip reader
	gzReader, err := gzip.NewReader(gzFile)
	if err != nil {
		return fmt.Errorf("error creating gzip reader: %v", err)
	}
	defer gzReader.Close()

	// Create the output file
	outFile, err := os.Create("ip2asn-v4.tsv")
	if err != nil {
		return fmt.Errorf("error creating output file: %v", err)
	}
	defer outFile.Close()

	// Copy the uncompressed content to the output file
	_, err = io.Copy(outFile, gzReader)
	if err != nil {
		return fmt.Errorf("error extracting gzipped file: %v", err)
	}

	fmt.Println("Successfully downloaded and extracted ip2asn-v4.tsv")
	return nil
}

// loadASNEntries loads ASN entries from the TSV file
func loadASNEntries() error {
	// Open the TSV file
	file, err := os.Open("ip2asn-v4.tsv")
	if err != nil {
		return fmt.Errorf("error opening ip2asn-v4.tsv: %v", err)
	}
	defer file.Close()

	// Create a scanner to read the file line by line
	scanner := bufio.NewScanner(file)

	// Read each line and parse it
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, "\t")

		// Ensure the line has the expected number of fields
		if len(fields) >= 5 {
			entry := ASNEntry{
				RangeStart:    fields[0],
				RangeEnd:      fields[1],
				ASNumber:      fields[2],
				CountryCode:   fields[3],
				ASDescription: fields[4],
			}
			asnEntries = append(asnEntries, entry)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading ip2asn-v4.tsv: %v", err)
	}

	fmt.Printf("Loaded %d ASN entries from ip2asn-v4.tsv\n", len(asnEntries))
	return nil
}

// getIPRangesForASN fetches IP ranges for a given ASN using the local TSV file
func getIPRangesForASN(asn string) ([]IPRange, error) {
	// Remove "AS" prefix if present
	asnNumber := strings.TrimPrefix(asn, "AS")

	// Find all entries matching the ASN
	var ipRanges []IPRange
	for _, entry := range asnEntries {
		if entry.ASNumber == asnNumber {
			ipRanges = append(ipRanges, IPRange{
				Start: entry.RangeStart,
				End:   entry.RangeEnd,
			})
		}
	}

	if len(ipRanges) == 0 {
		return nil, fmt.Errorf("no IP ranges found for ASN %s", asn)
	}

	return ipRanges, nil
}

func main() {
	// Define command-line flags
	inputFile := flag.String("input", "cdn_asn.csv", "Path to the input CSV file (required)")
	outputFile := flag.String("output", "cdn_asn_to_ip_map.json", "Path to the output JSON file (required)")

	// Parse flags
	flag.Parse()

	// Validate flags
	if *inputFile == "" || *outputFile == "" {
		flag.Usage()
		log.Fatal("Both input and output file paths are required")
	}

	// Check if the output file already exists
	if _, err := os.Stat(*outputFile); err == nil {
		log.Fatalf("Output file %s already exists. Please specify a different output file to prevent overwriting, or delete existing file", *outputFile)
	}

	// Check for ip2asn-v4.tsv and download if needed
	if err := checkAndDownloadTSV(); err != nil {
		log.Fatalf("Error checking/downloading ip2asn-v4.tsv: %v", err)
	}

	// Load ASN entries from the TSV file
	if err := loadASNEntries(); err != nil {
		log.Fatalf("Error loading ASN entries: %v", err)
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
