package sqsutil

import "os"

const separator = "/"

// WriteBytes writes the given bytes to the given file in the given directory.
// If the directory does not exist, it is created.
// If the file already exists, it is overwritten.
// Returns an error if any.
func WriteBytes(directory, fileName string, bz []byte) error {
	// Create a directory if not exists
	err := os.MkdirAll(directory, os.ModePerm)
	if err != nil {
		return err
	}

	// Write the bytes to file
	file, err := os.Create(directory + separator + fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(bz)
	if err != nil {
		return err
	}
	return nil
}
