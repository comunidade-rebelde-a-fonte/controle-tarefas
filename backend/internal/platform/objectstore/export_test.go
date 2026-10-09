package objectstore

import (
	"context"
	"net/http"
	"net/url"
)

// RawDelete existe só nos testes: prova que a credencial da aplicação não consegue apagar o
// original (a política do MinIO nega), já que o cliente de produção não tem operação de apagar.
func RawDelete(ctx context.Context, s *Store, key, versionID string) error {
	q := url.Values{}
	if versionID != "" {
		q.Set("versionId", versionID)
	}
	resp, err := s.do(ctx, http.MethodDelete, key, q, nil, nil, EmptySHA256)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}
