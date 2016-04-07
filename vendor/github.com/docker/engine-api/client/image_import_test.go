package client

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"

	"golang.org/x/net/context"

	"github.com/docker/engine-api/client/transport"
	"github.com/docker/engine-api/types"
	"strings"
)

func TestImageImportError(t *testing.T) {
	client := &Client{
		transport: transport.NewMockClient(nil, transport.ErrorMock(http.StatusInternalServerError, "Server error")),
	}
	_, err := client.ImageImport(context.Background(), types.ImageImportOptions{})
	if err == nil || err.Error() != "Error response from daemon: Server error" {
		t.Fatalf("expected a Server error, got %v", err)
	}
}

func TestImageImport(t *testing.T) {
	expectedURL := "/images/create"
	client := &Client{
		transport: transport.NewMockClient(nil, func(r *http.Request) (*http.Response, error) {
			if !strings.HasPrefix(r.URL.Path, expectedURL) {
				return nil, fmt.Errorf("Expected URL '%s', got '%s'", expectedURL, r.URL)
			}
			query := r.URL.Query()
			fromSrc := query.Get("fromSrc")
			if fromSrc != "image_source" {
				return nil, fmt.Errorf("fromSrc not set in URL query properly. Expected 'image_source', got %s", fromSrc)
			}
			repo := query.Get("repo")
			if repo != "repository_name" {
				return nil, fmt.Errorf("repo not set in URL query properly. Expected 'repository_name', got %s", repo)
			}
			tag := query.Get("tag")
			if tag != "imported" {
				return nil, fmt.Errorf("tag not set in URL query properly. Expected 'imported', got %s", tag)
			}
			message := query.Get("message")
			if message != "A message" {
				return nil, fmt.Errorf("message not set in URL query properly. Expected 'A message', got %s", message)
			}
			changes := query["changes"]
			expectedChanges := []string{"change1", "change2"}
			if !reflect.DeepEqual(expectedChanges, changes) {
				return nil, fmt.Errorf("changes not set in URL query properly. Expected %v, got %v", expectedChanges, changes)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(bytes.NewReader([]byte("response"))),
			}, nil
		}),
	}
	importResponse, err := client.ImageImport(context.Background(), types.ImageImportOptions{
		Source:         strings.NewReader("source"),
		SourceName:     "image_source",
		RepositoryName: "repository_name",
		Message:        "A message",
		Tag:            "imported",
		Changes:        []string{"change1", "change2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := ioutil.ReadAll(importResponse)
	if err != nil {
		t.Fatal(err)
	}
	importResponse.Close()
	if string(response) != "response" {
		t.Fatalf("expected response to contain 'response', got %s", string(response))
	}
}
