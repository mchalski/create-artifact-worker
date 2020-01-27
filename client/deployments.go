// Copyright 2020 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.
package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	uriInternalUpload = "/api/management/v1/deployments/tenants/{id}/artifacts"
)

var (
	timeoutSec = 5 * time.Second
)

type Deployments interface {
	UploadArtifactInternal(ctx context.Context, path, aid, tid, desc string) error
}

type deployments struct {
	deplUrl string
	c       *http.Client
}

func NewDeployments(deplUrl string, skipSsl bool) (Deployments, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: skipSsl,
		},
	}

	c := &http.Client{
		Transport: tr,
	}

	return &deployments{
		deplUrl: deplUrl,
		c:       c,
	}, nil
}

func (d *deployments) UploadArtifactInternal(ctx context.Context, fpath, aid, tid, desc string) error {
	ctx, cancel := context.WithTimeout(ctx, timeoutSec)
	defer cancel()

	artifact, err := os.Open(fpath)
	if err != nil {
		return errors.Wrapf(err, "cannot read artifact file %s", fpath)
	}
	defer artifact.Close()

	body := &bytes.Buffer{}

	writer := multipart.NewWriter(body)

	writer.WriteField("id", tid)
	writer.WriteField("artifact_id", aid)
	writer.WriteField("description", desc)

	part, err := writer.CreateFormFile("artifact", filepath.Base(fpath))

	_, err = io.Copy(part, artifact)

	err = writer.Close()
	if err != nil {
		return errors.Wrap(err, "cannot create artifact upload request")
	}

	req, err := http.NewRequest(http.MethodPost,
		path.Join(d.deplUrl, strings.Replace(uriInternalUpload, "{id}", tid, 1)),
		body)
	if err != nil {
		return errors.Wrap(err, "cannot create artifact upload request")
	}

	req = req.WithContext(ctx)

	req.Header.Set("Content-Type", writer.FormDataContentType())

	res, err := d.c.Do(req)

	if res.StatusCode != http.StatusCreated {
		return errors.Wrapf(apiErr(res), "failed to upload artifact %s", aid)
	}

	return nil

}

func apiErr(r *http.Response) error {
	e := struct {
		Reqid string `json:"request_id"`
		Msg   string `json:"error"`
	}{}

	err := json.NewDecoder(r.Body).Decode(&e)
	if err != nil {
		return errors.New(fmt.Sprintf("got error code %d from api but failed to decode response", r.StatusCode))
	}

	return errors.New(fmt.Sprintf("http %s reqid %d msg %s ", e.Reqid, r.StatusCode, e.Msg))
}
