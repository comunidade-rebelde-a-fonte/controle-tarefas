package objectstore

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

// Exemplos da documentação da AWS ("Signature Calculations for the Authorization Header:
// Transferring Payload in a Single Chunk"), com as credenciais de exemplo publicadas nela.
const (
	exampleAccessKey = "AKIAIOSFODNN7EXAMPLE"
	exampleSecretKey = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
)

var exampleTime = time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)

func TestSigV4_AWSExampleGetObject(t *testing.T) {
	r := signingRequest{
		method: "GET",
		path:   encodeURIPath("/test.txt"),
		query:  url.Values{},
		headers: map[string]string{
			"host":                 "examplebucket.s3.amazonaws.com",
			"range":                "bytes=0-9",
			"x-amz-content-sha256": EmptySHA256,
			"x-amz-date":           "20130524T000000Z",
		},
		payloadHash: EmptySHA256,
	}
	creq, _ := canonicalRequest(r)
	if got := sha256Hex(creq); got != "7344ae5b7ee6c3e7e6b0fe0640412a37625d1fbfff95c48bbb2dc43964946972" {
		t.Fatalf("hash da requisição canônica = %s\n%s", got, creq)
	}
	auth := authorization(r, exampleAccessKey, exampleSecretKey, "us-east-1", exampleTime)
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, " +
		"SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, " +
		"Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if auth != want {
		t.Fatalf("Authorization =\n%s\nquer\n%s", auth, want)
	}
}

func TestSigV4_AWSExamplePutObject(t *testing.T) {
	body := "Welcome to Amazon S3."
	r := signingRequest{
		method: "PUT",
		path:   encodeURIPath("/test$file.text"),
		query:  url.Values{},
		headers: map[string]string{
			"date":                 "Fri, 24 May 2013 00:00:00 GMT",
			"host":                 "examplebucket.s3.amazonaws.com",
			"x-amz-content-sha256": sha256Hex(body),
			"x-amz-date":           "20130524T000000Z",
			"x-amz-storage-class":  "REDUCED_REDUNDANCY",
		},
		payloadHash: sha256Hex(body),
	}
	if r.path != "/test%24file.text" {
		t.Fatalf("caminho = %s", r.path)
	}
	auth := authorization(r, exampleAccessKey, exampleSecretKey, "us-east-1", exampleTime)
	if !strings.HasSuffix(auth, "Signature=98ad721746da40c64f1a55b78f14c238d841ea1380cd77a1b5971af0ece108bd") {
		t.Fatalf("Authorization = %s", auth)
	}
}

func TestCanonicalQuery_SortedAndEncoded(t *testing.T) {
	q := url.Values{"versionId": {"a b/c"}, "acl": {""}}
	if got := canonicalQuery(q); got != "acl=&versionId=a%20b%2Fc" {
		t.Fatalf("query = %s", got)
	}
}
