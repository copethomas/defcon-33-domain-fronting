package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type TestCase struct {
	TargetDomain string `json:"target_domain"`
	FrontDomain  string `json:"front_domain"`
	TargetUrl    string `json:"target_url"`
	CDN          string `json:"cdn"`
}

type TestResult struct {
	TestCase    TestCase
	ReturnCode  int
	Stdout      string
	Stderr      string
	ElapsedTime time.Duration
}

func main() {
	// Parse command line flags
	jsonFilePath := flag.String("json", "test_cases.json", "Path to the JSON file containing test cases")
	pythonScript := flag.String("script", "", "Path to the Python script to execute")
	numWorkers := flag.Int("workers", 10, "Number of worker threads to use")
	flag.Parse()

	// Validate flags
	if *pythonScript == "" {
		fmt.Println("Error: Please provide a Python script path using the -script flag")
		os.Exit(1)
	}

	// Set up zerolog
	// Create a timestamp for the log file name
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	logFileName := fmt.Sprintf("test_runner_%s.log", timestamp)

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
	jsonFile, err := os.Open(*jsonFilePath)
	if err != nil {
		log.Error().Err(err).Str("file", *jsonFilePath).Msg("Failed to open JSON file")
		os.Exit(1)
	}
	defer jsonFile.Close()

	// Read file contents
	jsonData, err := io.ReadAll(jsonFile)
	if err != nil {
		log.Error().Err(err).Str("file", *jsonFilePath).Msg("Failed to read JSON file")
		os.Exit(1)
	}

	// Parse JSON data
	var testCases []TestCase
	err = json.Unmarshal(jsonData, &testCases)
	if err != nil {
		log.Error().Err(err).Str("file", *jsonFilePath).Msg("Failed to parse JSON data")
		os.Exit(1)
	}

	log.Info().Int("test_cases", len(testCases)).Msg("Loaded test cases from JSON file")

	// Create a channel for test cases
	testCaseChan := make(chan TestCase, len(testCases))
	resultChan := make(chan TestResult, len(testCases))

	// Start worker pool
	var wg sync.WaitGroup
	for i := 0; i < *numWorkers; i++ {
		wg.Add(1)
		go worker(i, *pythonScript, testCaseChan, resultChan, &wg)
	}

	// Send test cases to the channel
	for _, tc := range testCases {
		testCaseChan <- tc
	}
	close(testCaseChan)

	// Create a goroutine to close the result channel when all workers are done
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Process results
	successCount := 0
	failureCount := 0
	for result := range resultChan {
		if result.ReturnCode == 0 {
			successCount++
			log.Info().
				Str("target_domain", result.TestCase.TargetDomain).
				Str("front_domain", result.TestCase.FrontDomain).
				Str("target_url", result.TestCase.TargetUrl).
				Str("cdn", result.TestCase.CDN).
				Int("return_code", result.ReturnCode).
				Str("stdout", result.Stdout).
				Dur("elapsed_time", result.ElapsedTime).
				Msg("Test case succeeded")
		} else {
			failureCount++
			log.Error().
				Str("target_domain", result.TestCase.TargetDomain).
				Str("front_domain", result.TestCase.FrontDomain).
				Str("target_url", result.TestCase.TargetUrl).
				Str("cdn", result.TestCase.CDN).
				Int("return_code", result.ReturnCode).
				Str("stdout", result.Stdout).
				Str("stderr", result.Stderr).
				Dur("elapsed_time", result.ElapsedTime).
				Msg("Test case failed")
		}
	}

	log.Info().
		Int("total", len(testCases)).
		Int("success", successCount).
		Int("failure", failureCount).
		Msg("Test execution completed")
}

func worker(id int, pythonScript string, testCases <-chan TestCase, results chan<- TestResult, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Debug().Int("worker_id", id).Msg("Worker started")

	for tc := range testCases {
		startTime := time.Now()

		// Prepare command
		cmd := exec.Command("python", pythonScript,
			"--target-domain", tc.TargetDomain,
			"--front-domain", tc.FrontDomain,
			"--target-url", tc.TargetUrl,
			"--cdn", tc.CDN)

		// Capture stdout and stderr
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		// Execute command
		err := cmd.Run()

		// Calculate elapsed time
		elapsedTime := time.Since(startTime)

		// Determine return code
		returnCode := 0
		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				returnCode = exitError.ExitCode()
			} else {
				returnCode = -1
				log.Error().Err(err).Msg("Failed to execute Python script")
			}
		}

		// Send result
		results <- TestResult{
			TestCase:    tc,
			ReturnCode:  returnCode,
			Stdout:      stdout.String(),
			Stderr:      stderr.String(),
			ElapsedTime: elapsedTime,
		}

		log.Debug().
			Int("worker_id", id).
			Str("target_domain", tc.TargetDomain).
			Int("return_code", returnCode).
			Dur("elapsed_time", elapsedTime).
			Msg("Processed test case")
	}

	log.Debug().Int("worker_id", id).Msg("Worker finished")
}
