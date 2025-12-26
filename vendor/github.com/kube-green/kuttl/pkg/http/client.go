package http

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/dustin/go-humanize"

	"github.com/kube-green/kuttl/pkg/version"
)

// Client is client used to simplified http requests for tarballs
type Client struct {
	client    *http.Client
	UserAgent string
}

// Get performs an HTTP get and returns the Response.  The caller is responsible for closing the response.
func (c *Client) Get(url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.client.Do(req)
	return resp, err
}

// Get performs HTTP get, retrieves the contents and returns a bytes.Buffer of the entire contents
// this could be dangerous against an extremely large file
func (c *Client) GetByteBuffer(url string) (*bytes.Buffer, error) {
	buf := bytes.NewBuffer(nil)

	resp, err := c.Get(url)
	if err != nil {
		return buf, err
	}
	if resp.StatusCode != http.StatusOK {
		return buf, fmt.Errorf("failed to fetch %s : %s", url, resp.Status)
	}

	_, err = io.Copy(buf, resp.Body)
	if err != nil {
		return nil, err
	}
	err = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("error when closing the response body %s", err)
	}
	return buf, err
}

// DownloadFile expects a url with a file and will save that file to the path provided preserving the file name.
func (c *Client) DownloadFile(url, path string) (string, error) {
	filePath := filepath.Join(path, filepath.Base(url))
	return filePath, c.Download(url, filePath)
}

// Download takes a url to download and a filepath to write it to
// this will write the response to any url request to a file.
func (c *Client) Download(url string, path string) error {
	// Get the data
	resp, err := c.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = os.Stat(path)
	if err == nil || os.IsExist(err) {
		return os.ErrExist
	}
	// captures errors other than does not exist
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Create the file with .tmp extension, so that we won't overwrite a
	// file until it's downloaded fully
	out, err := os.Create(path + ".tmp")
	if err != nil {
		return err
	}
	defer out.Close()

	// Create our bytes counter and pass it to be used alongside our writer
	counter := &writeCounter{Name: filepath.Base(path)}
	_, err = io.Copy(out, io.TeeReader(resp.Body, counter))
	if err != nil {
		return err
	}

	// The progress use the same line so print a new line once it's finished downloading
	fmt.Println()

	// Rename the tmp file back to the original file
	err = os.Rename(path+".tmp", path)
	if err != nil {
		return err
	}

	return nil
}

type writeCounter struct {
	Total uint64
	Name  string
}

func (wc *writeCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Total += uint64(n)
	wc.PrintProgress()
	return n, nil
}

// PrintProgress prints the progress of a file write
func (wc *writeCounter) PrintProgress() {
	// Clear the line by using a character return to go back to the start and remove
	// the remaining characters by filling it with spaces
	fmt.Printf("\r%s", strings.Repeat(" ", 100))

	// Return again and print current status of download
	// We use the humanize package to print the bytes in a meaningful way (e.g. 10 MB)
	fmt.Printf("\rDownloading (%s) %s complete", wc.Name, humanize.Bytes(wc.Total))
}

// NewClient creates HTTP client
func NewClient() *Client {
	var client Client
	tr := &http.Transport{
		DisableCompression: true,
		Proxy:              http.ProxyFromEnvironment,
	}

	client.client = &http.Client{Transport: tr}
	client.UserAgent = fmt.Sprintf("KUTTL/%s", strings.TrimPrefix(version.Get().GitVersion, "v"))
	return &client
}
