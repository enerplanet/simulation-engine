package utils

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ToStr converts interface{} to string (handles both string and numeric types)
func ToStr(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%.0f", val)
	case int:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// LogFatal prints errors and exits
func LogFatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// Log prints errors
func Log(err error) {
	if err != nil {
		log.Print(err)
	}
}

// CleanYAML cleans up the YAML string
func CleanYAML(str string) string {
	re := regexp.MustCompile(`(?m)^[ \t]*available_area: -2(?:\.0+)?\r?\n`)
	cleaned := re.ReplaceAllString(str, "")
	cleaned = strings.ReplaceAll(cleaned, `"y":`, `y:`)
	cleaned = strings.ReplaceAll(cleaned, " {}", "")
	cleaned = strings.ReplaceAll(cleaned, " null", "")

	return cleaned
}

// CreateDir creates a new named dir if possible
func CreateDir(name string) error {
	_, err := os.Stat(name)
	if os.IsNotExist(err) {
		err = os.MkdirAll(name, 0755)
	}

	return err
}

// RemoveDir creates a new named dir if possible
func RemoveDir(name string) error {
	_, err := os.Stat(name)
	if !os.IsNotExist(err) {
		err = os.RemoveAll(name)
	}

	return err
}

// CreateFile creates a file
func CreateFile(name string, content string) error {
	_, err := os.Stat(name)
	if os.IsNotExist(err) {
		f, err := os.Create(name)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = f.WriteString(content)
		if err != nil {
			return err
		}

		return err
	}

	return err
}

// ReadFile reads a file
func ReadFile(name string) (string, error) {
	_, err := os.Stat(name)
	if os.IsNotExist(err) {
		return "", err
	}

	content, err := ioutil.ReadFile(name)
	if err != nil {
		return "", err
	}

	return string(content), err
}

// ReadCSVFile reads csv file
func ReadCSVFile(name string) [][]string {
	file, err := os.OpenFile(name, os.O_RDONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	r := csv.NewReader(file)

	data, err := r.ReadAll()
	LogFatal(err)

	return data
}

// CopyTemplateFile duplicates a template file
func CopyTemplateFile(name string, from string, to string) {
	content, err := ReadFile(from + "/" + name)
	if err != nil {
		return
	}

	if err := CreateFile(to+"/"+name, content); err != nil {
		return
	}
}

// ZipFolder archives a folder recusively
func ZipFolder(from, to string) error {
	if _, err := os.Stat(from); os.IsNotExist(err) {
		return err
	}

	if _, err := os.Stat(to); err == nil {
		return os.ErrExist
	}

	file, err := os.Create(to)
	if err != nil {
		return err
	}
	defer file.Close()

	zipped := zip.NewWriter(file)

	if err := filepath.Walk(from, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info == nil {
			return fmt.Errorf("zip walk returned nil file info for path %q", path)
		}
		if info.IsDir() {
			return nil
		}

		// Keep archive entries relative and stable, including the simulation folder name.
		relativePath, err := filepath.Rel(filepath.Dir(from), path)
		if err != nil {
			return err
		}
		relativePath = filepath.ToSlash(relativePath)
		relativePath = strings.TrimPrefix(relativePath, "./")
		relativePath = strings.TrimPrefix(relativePath, "/")
		if relativePath == "" || strings.HasPrefix(relativePath, "../") {
			return fmt.Errorf("invalid archive path derived from %q: %q", path, relativePath)
		}

		archive, err := zipped.Create(relativePath)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(archive, file)
		if err != nil {
			return err
		}

		return nil
	}); err != nil {
		return err
	}

	if err := zipped.Close(); err != nil {
		return err
	}

	return nil
}

// SendZip sends a zip-file to a path
func SendZip(path, hexID, userID, modelID, pypsa, file, callbackSecret string) error {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	fw, err := writer.CreateFormField("hex_id")
	if err != nil {
		return err
	}

	if _, err = io.Copy(fw, strings.NewReader(hexID)); err != nil {
		return err
	}

	fw, err = writer.CreateFormField("user_id")
	if err != nil {
		return err
	}

	if _, err = io.Copy(fw, strings.NewReader(userID)); err != nil {
		return err
	}

	fw, err = writer.CreateFormField("model_id")
	if err != nil {
		return err
	}

	if _, err := io.Copy(fw, strings.NewReader(modelID)); err != nil {
		return err
	}

	fw, err = writer.CreateFormField("pypsa")
	if err != nil {
		return err
	}

	if _, err = io.Copy(fw, strings.NewReader(pypsa)); err != nil {
		return err
	}

	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return err
	}

	fw, err = writer.CreateFormFile("file", fi.Name())
	if err != nil {
		return err
	}

	if _, err := io.Copy(fw, f); err != nil {
		return err
	}

	writer.Close()

	contentType := writer.FormDataContentType()
	bodyBytes := body.Bytes()

	client := &http.Client{
		Timeout: 0, // no timeout; large result zips can take a long time to upload
	}

	const maxRetries = 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest(http.MethodPost, path, bytes.NewReader(bodyBytes))
		if err != nil {
			return err
		}

		req.Header.Set("Content-Type", contentType)
		if callbackSecret != "" {
			req.Header.Set("X-Callback-Secret", callbackSecret)
		}

		rsp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("callback request to %s failed (attempt %d/%d): %w", path, attempt, maxRetries, err)
			if attempt < maxRetries {
				backoff := time.Duration(1<<uint(attempt-1)) * 5 * time.Second // 5s, 10s
				time.Sleep(backoff)
			}
			continue
		}

		respBody, _ := io.ReadAll(io.LimitReader(rsp.Body, 4096))
		rsp.Body.Close()
		respBodyStr := strings.TrimSpace(string(respBody))

		if rsp.StatusCode == http.StatusOK {
			return nil
		}

		if respBodyStr == "" {
			lastErr = fmt.Errorf("request failed with response code: %d (attempt %d/%d)", rsp.StatusCode, attempt, maxRetries)
		} else {
			lastErr = fmt.Errorf("request failed with response code: %d, body: %s (attempt %d/%d)", rsp.StatusCode, respBodyStr, attempt, maxRetries)
		}

		// Only retry on server errors (5xx), not client errors (4xx)
		if rsp.StatusCode >= 400 && rsp.StatusCode < 500 {
			return lastErr
		}

		if attempt < maxRetries {
			backoff := time.Duration(1<<uint(attempt-1)) * 5 * time.Second
			time.Sleep(backoff)
		}
	}

	return lastErr
}

// ExecCMD executes a command string asynchronously.
// The command runs in the background; a goroutine waits for it to finish
// and logs any errors. Use ExecCMDWithSemaphore to enforce concurrency limits.
func ExecCMD(cmdString string) error {
	cmd := exec.Command("bash", "-c", cmdString)

	err := cmd.Start()
	if err != nil {
		return err
	}

	// Wait for the process to finish in a background goroutine
	go func() {
		if waitErr := cmd.Wait(); waitErr != nil {
			log.Printf("Command finished with error: %v (cmd: %s)", waitErr, cmdString)
		}
	}()

	return nil
}

// ExecCMDWithSemaphore executes a command string asynchronously while holding
// a semaphore slot until the command completes. This ensures the concurrency
// limit is respected for the full duration of the command, not just its launch.
func ExecCMDWithSemaphore(cmdString string, sem chan struct{}) error {
	cmd := exec.Command("bash", "-c", cmdString)

	err := cmd.Start()
	if err != nil {
		return err
	}

	// Wait for the process to finish in a background goroutine,
	// then release the semaphore slot
	go func() {
		if waitErr := cmd.Wait(); waitErr != nil {
			log.Printf("Command finished with error: %v (cmd: %s)", waitErr, cmdString)
		}
		// Release the semaphore slot when the command finishes
		<-sem
	}()

	return nil
}

func ExecArgvWithSemaphore(binary string, args []string, sem chan struct{}) error {
	cmd := exec.Command(binary, args...)

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		if waitErr := cmd.Wait(); waitErr != nil {
			log.Printf("Command finished with error: %v (binary: %s args: %v)", waitErr, binary, args)
		}
		<-sem
	}()

	return nil
}

// SendFailureCallback sends a failure status to the platform callback URL
// so the model is marked as failed instead of staying in "calculating".
func SendFailureCallback(callbackURL, userID, modelID, errorMessage, callbackSecret string) error {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	fields := map[string]string{
		"user_id":       userID,
		"model_id":      modelID,
		"status":        "failed",
		"error_message": errorMessage,
	}

	for key, val := range fields {
		fw, err := writer.CreateFormField(key)
		if err != nil {
			return fmt.Errorf("failed to create form field %s: %w", key, err)
		}
		if _, err = io.Copy(fw, strings.NewReader(val)); err != nil {
			return fmt.Errorf("failed to write form field %s: %w", key, err)
		}
	}

	writer.Close()

	client := &http.Client{Timeout: 30 * time.Second}

	const maxRetries = 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest(http.MethodPost, callbackURL, bytes.NewReader(body.Bytes()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())
		if callbackSecret != "" {
			req.Header.Set("X-Callback-Secret", callbackSecret)
		}

		rsp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failure callback to %s failed (attempt %d/%d): %w", callbackURL, attempt, maxRetries, err)
			if attempt < maxRetries {
				time.Sleep(time.Duration(1<<uint(attempt-1)) * 5 * time.Second)
			}
			continue
		}
		rsp.Body.Close()

		if rsp.StatusCode == http.StatusOK {
			return nil
		}

		lastErr = fmt.Errorf("failure callback returned status %d (attempt %d/%d)", rsp.StatusCode, attempt, maxRetries)
		if rsp.StatusCode >= 400 && rsp.StatusCode < 500 {
			return lastErr
		}

		if attempt < maxRetries {
			time.Sleep(time.Duration(1<<uint(attempt-1)) * 5 * time.Second)
		}
	}

	return lastErr
}
