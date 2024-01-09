package sqsutil_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/osmosis-labs/sqs/sqsutil"
)

// This test is ChatGPT generated with the following prompt:
//
// Please generate table driven unit tests for this function.
//
// Make sure to test:
// - New file is written without issues
// - The contents of the file are overwritten if previously exist
// - If directory exists, no error returned
// - If directory does not exist, it is created and no error returned
// - Ability to write twice to the same file without permission issues
func TestWriteBytes(t *testing.T) {
	tests := []struct {
		name      string
		directory string
		fileName  string
		data      []byte
		setup     func() // setup function for any required preconditions
		assert    func() // assert function for validating the result
		cleanup   func() // cleanup function for any necessary cleanup after the test
	}{
		{
			name:      "New file is written without issues",
			directory: "testdata/test1",
			fileName:  "file1.txt",
			data:      []byte("Hello, World!"),
			setup: func() {
				// No setup required for this test case
			},
			assert: func() {
				// Check if the file was created
				fileContent, err := os.ReadFile("testdata/test1/file1.txt")
				if err != nil {
					t.Errorf("Error reading file: %v", err)
				}
				expectedContent := "Hello, World!"
				if string(fileContent) != expectedContent {
					t.Errorf("Unexpected file content. Expected: %s, Actual: %s", expectedContent, fileContent)
				}
			},
			cleanup: func() {
				// Cleanup: Remove the directory and its contents
				os.RemoveAll("testdata/test1")
			},
		},
		{
			name:      "Contents of the file are overwritten if previously exist",
			directory: "testdata/test2",
			fileName:  "file2.txt",
			data:      []byte("Hello, World!"),
			setup: func() {
				// Create the directory
				err := os.MkdirAll("testdata/test2", os.ModePerm)
				if err != nil {
					t.Fatalf("Setup error: %v", err)
				}

				// Create a file with initial content
				err = os.WriteFile("testdata/test2/file2.txt", []byte("Initial Content"), 0644)
				if err != nil {
					t.Fatalf("Setup error: %v", err)
				}
			},
			assert: func() {
				// Check if the file was overwritten
				fileContent, err := os.ReadFile("testdata/test2/file2.txt")
				if err != nil {
					t.Errorf("Error reading file: %v", err)
				}
				expectedContent := "Hello, World!"
				if string(fileContent) != expectedContent {
					t.Errorf("Unexpected file content. Expected: %s, Actual: %s", expectedContent, fileContent)
				}
			},
			cleanup: func() {
				// Cleanup: Remove the directory and its contents
				os.RemoveAll("testdata/test2")
			},
		},
		{
			name:      "If directory exists, no error returned",
			directory: "testdata/test3",
			fileName:  "file3.txt",
			data:      []byte("Hello, World!"),
			setup: func() {
				// Create the directory
				err := os.MkdirAll("testdata/test3", os.ModePerm)
				if err != nil {
					t.Fatalf("Setup error: %v", err)
				}
			},
			assert: func() {
				// Check if the file was created
				fileContent, err := os.ReadFile("testdata/test3/file3.txt")
				if err != nil {
					t.Errorf("Error reading file: %v", err)
				}
				expectedContent := "Hello, World!"
				if string(fileContent) != expectedContent {
					t.Errorf("Unexpected file content. Expected: %s, Actual: %s", expectedContent, fileContent)
				}
			},
			cleanup: func() {
				// Cleanup: Remove the directory and its contents
				os.RemoveAll("testdata/test3")
			},
		},
		{
			name:      "If directory does not exist, it is created and no error returned",
			directory: "testdata/test4",
			fileName:  "file4.txt",
			data:      []byte("Hello, World!"),
			setup: func() {
				// No setup required for this test case
			},
			assert: func() {
				// Check if the file was created
				fileContent, err := os.ReadFile("testdata/test4/file4.txt")
				if err != nil {
					t.Errorf("Error reading file: %v", err)
				}
				expectedContent := "Hello, World!"
				if string(fileContent) != expectedContent {
					t.Errorf("Unexpected file content. Expected: %s, Actual: %s", expectedContent, fileContent)
				}
			},
			cleanup: func() {
				// Cleanup: Remove the directory and its contents
				os.RemoveAll("testdata/test4")
			},
		},
		{
			name:      "Ability to write twice to the same file without permission issues",
			directory: "testdata/test5",
			fileName:  "file5.txt",
			data:      []byte("Hello, World!"),
			setup: func() {
				// No setup required for this test case
			},
			assert: func() {
				// Check if the file was created
				fileContent, err := ioutil.ReadFile("testdata/test5/file5.txt")
				if err != nil {
					t.Errorf("Error reading file: %v", err)
				}
				expectedContent := "Hello, World!"
				if string(fileContent) != expectedContent {
					t.Errorf("Unexpected file content. Expected: %s, Actual: %s", expectedContent, fileContent)
				}

				// Write to the same file again
				err = sqsutil.WriteBytes("testdata/test5", "file5.txt", []byte("New Content"))
				if err != nil {
					t.Errorf("Error writing to the same file: %v", err)
				}

				// Check if the file was overwritten
				fileContent, err = ioutil.ReadFile("testdata/test5/file5.txt")
				if err != nil {
					t.Errorf("Error reading file: %v", err)
				}
				expectedContent = "New Content"
				if string(fileContent) != expectedContent {
					t.Errorf("Unexpected file content. Expected: %s, Actual: %s", expectedContent, fileContent)
				}
			},
			cleanup: func() {
				// Cleanup: Remove the directory and its contents
				os.RemoveAll("testdata/test5")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run the setup function
			if tt.setup != nil {
				tt.setup()
			}

			// Run the function being tested
			err := sqsutil.WriteBytes(tt.directory, tt.fileName, tt.data)
			if err != nil {
				t.Fatalf("Error writing bytes to file: %v", err)
			}

			// Run the assert function
			tt.assert()

			// Run the cleanup function
			if tt.cleanup != nil {
				tt.cleanup()
			}
		})
	}
}
