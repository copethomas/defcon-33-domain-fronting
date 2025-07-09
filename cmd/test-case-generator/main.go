package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// CDNRecord represents a single record in the JSON array
type CDNRecord struct {
	CDN            string `json:"cdn"`
	VisitedDomain  string `json:"visited_domain"`
	OriginalDomain string `json:"original_domain"`
	ResourceURL    string `json:"resource_url"`
	ContentType    string `json:"content_type"`
	ServerIP       string `json:"server_ip"`
}
type TestCase struct {
	TargetDomain string `json:"target_domain"`
	FrontDomain  string `json:"front_domain"`
	TargetUrl    string `json:"target_url"`
	CDN          string `json:"cdn"`
}

func main() {
	// Parse command line flags
	filePath := flag.String("file", "resources_details.json", "Path to the JSON file to process")
	testCaseOutputPath := flag.String("output", "test_cases.json", "Path to write the generated test cases")
	forceOverwrite := flag.Bool("force", false, "Force overwrite of output file if it already exists")
	flag.Parse()

	// Check if output file already exists
	if _, err := os.Stat(*testCaseOutputPath); err == nil {
		// File exists
		if !*forceOverwrite {
			log.Error().Str("file", *testCaseOutputPath).Msg("Output file already exists. Use -force flag to overwrite or specify a different output path")
			os.Exit(1)
		}
		log.Warn().Str("file", *testCaseOutputPath).Msg("Overwriting existing output file because -force flag was set")
	}

	if *filePath == "" {
		fmt.Println("Error: Please provide a file path using the -file flag")
		os.Exit(1)
	}

	if *testCaseOutputPath == "" {
		fmt.Println("Error: Please provide an output file path using the -output flag")
		os.Exit(1)
	}

	// Set up zerolog
	// Create a timestamp for the log file name
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	logFileName := fmt.Sprintf("cdn_records_%s.log", timestamp)

	// Create log file
	logFile, err := os.Create(logFileName)
	if err != nil {
		fmt.Printf("Error creating log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	// Configure zerolog to write to both console and file
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	multi := zerolog.MultiLevelWriter(consoleWriter, logFile)
	log.Logger = zerolog.New(multi).With().Timestamp().Logger()

	// Read the JSON file
	jsonFile, err := os.Open(*filePath)
	if err != nil {
		log.Error().Err(err).Str("file", *filePath).Msg("Failed to open JSON file")
		os.Exit(1)
	}
	defer jsonFile.Close()

	// Read file contents
	jsonData, err := io.ReadAll(jsonFile)
	if err != nil {
		log.Error().Err(err).Str("file", *filePath).Msg("Failed to read JSON file")
		os.Exit(1)
	}

	// Parse JSON data
	var records []CDNRecord
	err = json.Unmarshal(jsonData, &records)
	if err != nil {
		log.Error().Err(err).Str("file", *filePath).Msg("Failed to parse JSON data")
		os.Exit(1)
	}

	// Group records by CDN
	cdnBuckets := make(map[string][]CDNRecord)
	for _, record := range records {
		// Parse the resource URL to check the path
		parsedURL, err := url.Parse(record.ResourceURL)
		if err != nil {
			log.Warn().Err(err).Str("resource_url", record.ResourceURL).Msg("Failed to parse resource URL")
			continue
		}

		// Skip records where the path is just "/" or empty
		if parsedURL.Path == "" || parsedURL.Path == "/" {
			log.Debug().Str("resource_url", record.ResourceURL).Msg("Skipping record with empty or root path")
			continue
		}

		cdnBuckets[record.CDN] = append(cdnBuckets[record.CDN], record)
	}

	// Generate test cases from CDN buckets
	var testCases []TestCase
	testCaseMap := make(map[string]bool)   // To track unique test cases
	domainPairMap := make(map[string]bool) // To track used front_domain and target_domain pairs

	log.Info().Msg("Generating test cases from CDN buckets")
	for cdn, bucket := range cdnBuckets {
		log.Info().Str("cdn", cdn).Int("record_count", len(bucket)).Msg("Generating test cases for CDN")

		// Create a map to track which records have been used
		usedRecords := make(map[int]bool)

		// Loop through each record in the bucket
		for i := 0; i < len(bucket); i++ {
			// Skip if this record has already been used
			if usedRecords[i] {
				continue
			}

			record := bucket[i]

			// Find a matching record with a different visited domain
			for j := 0; j < len(bucket); j++ {
				// Skip if this record has already been used or it's the same record or has the same visited domain
				if usedRecords[j] || i == j || record.VisitedDomain == bucket[j].VisitedDomain {
					continue
				}

				otherRecord := bucket[j]

				// Create a domain pair key to check if this pair has been used
				domainPairKey := fmt.Sprintf("%s|%s", record.VisitedDomain, otherRecord.VisitedDomain)

				// Skip if this domain pair has already been used
				if domainPairMap[domainPairKey] {
					log.Debug().
						Str("target_domain", record.VisitedDomain).
						Str("front_domain", otherRecord.VisitedDomain).
						Msg("Skipping already used domain pair")
					continue
				}

				// Create a test case
				testCase := TestCase{
					TargetDomain: record.VisitedDomain,
					FrontDomain:  otherRecord.VisitedDomain,
					TargetUrl:    record.ResourceURL,
					CDN:          cdn,
				}

				// Create a unique key for the test case to avoid duplicates
				key := fmt.Sprintf("%s|%s|%s", testCase.TargetDomain, testCase.FrontDomain, testCase.TargetUrl)

				// Add to test cases if not a duplicate
				if !testCaseMap[key] {
					testCases = append(testCases, testCase)
					testCaseMap[key] = true

					// Mark this domain pair as used
					domainPairMap[domainPairKey] = true

					// Mark both records as used
					usedRecords[i] = true
					usedRecords[j] = true

					log.Debug().
						Str("target_domain", testCase.TargetDomain).
						Str("front_domain", testCase.FrontDomain).
						Str("target_url", testCase.TargetUrl).
						Msg("Generated test case")

					// Break out of the inner loop since we've used this record
					break
				}
			}
		}

		// Create a new bucket without the used records
		var newBucket []CDNRecord
		for i, record := range bucket {
			if !usedRecords[i] {
				newBucket = append(newBucket, record)
			}
		}

		// Update the cdnBuckets map with the new bucket
		cdnBuckets[cdn] = newBucket

		log.Info().
			Str("cdn", cdn).
			Int("original_count", len(bucket)).
			Int("remaining_count", len(newBucket)).
			Int("removed_count", len(bucket)-len(newBucket)).
			Msg("Removed used records from CDN bucket")
	}

	log.Info().
		Int("total_test_cases", len(testCases)).
		Int("unique_domain_pairs", len(domainPairMap)).
		Msg("Test cases generated")

	// Write test cases to the specified output file
	testCaseFile, err := os.Create(*testCaseOutputPath)
	if err != nil {
		log.Error().Err(err).Str("file", *testCaseOutputPath).Msg("Failed to create test case output file")
		os.Exit(1)
	}
	defer testCaseFile.Close()

	// Marshal the test cases to JSON
	testCaseJSON, err := json.MarshalIndent(testCases, "", "  ")
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal test cases to JSON")
		os.Exit(1)
	}

	// Write the JSON data to the file
	_, err = testCaseFile.Write(testCaseJSON)
	if err != nil {
		log.Error().Err(err).Str("file", *testCaseOutputPath).Msg("Failed to write test cases to output file")
		os.Exit(1)
	}

	// Write the grouped records to a JSON file (original functionality)
	outputFileName := fmt.Sprintf("cdn_buckets_%s.json", timestamp)
	outputFile, err := os.Create(outputFileName)
	if err != nil {
		log.Error().Err(err).Str("file", outputFileName).Msg("Failed to create output file")
		os.Exit(1)
	}
	defer outputFile.Close()

	// Marshal the map to JSON
	jsonData, err = json.MarshalIndent(cdnBuckets, "", "  ")
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal CDN buckets to JSON")
		os.Exit(1)
	}

	// Write the JSON data to the file
	_, err = outputFile.Write(jsonData)
	if err != nil {
		log.Error().Err(err).Str("file", outputFileName).Msg("Failed to write to output file")
		os.Exit(1)
	}

	log.Info().
		Str("log_file", logFileName).
		Str("json_file", outputFileName).
		Str("test_case_file", *testCaseOutputPath).
		Int("test_case_count", len(testCases)).
		Msg("Records have been processed and test cases have been generated")
}
