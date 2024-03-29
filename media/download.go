/*
 * Copyright (c) 2021 Serena Tiede
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"

	"github.com/LadySerena/pi-image-builder/telemetry"
	"github.com/LadySerena/pi-image-builder/utility"
	"github.com/spf13/afero"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/sync/errgroup"
)

func DownloadAndVerifyMedia(ctx context.Context, fileSystem afero.Fs, forceOverwrite bool) error {

	ctx, span := telemetry.GetTracer().Start(ctx, "download media")
	defer span.End()

	releaseURL, parseErr := url.Parse("https://cdimage.ubuntu.com/releases/20.04/release")
	if parseErr != nil {
		return parseErr
	}

	checksumName := "SHA256SUMS"

	_, mediaStatErr := fileSystem.Stat(utility.ImageName)
	_, checksumStatErr := fileSystem.Stat(checksumName)

	group := new(errgroup.Group)
	group.Go(func() error {
		if forceOverwrite || errors.Is(mediaStatErr, fs.ErrNotExist) {
			mediaURL := *releaseURL
			mediaURL.Path = path.Join(releaseURL.Path, utility.ImageName)
			return DownloadFile(ctx, fileSystem, utility.ImageName, mediaURL.String())
		}
		return nil
	})
	group.Go(func() error {
		if forceOverwrite || errors.Is(checksumStatErr, fs.ErrNotExist) {
			checksumURL := *releaseURL
			checksumURL.Path = path.Join(releaseURL.Path, checksumName)
			return DownloadFile(ctx, fileSystem, checksumName, checksumURL.String())
		}
		return nil
	})
	if waitErr := group.Wait(); waitErr != nil {
		return waitErr
	}

	media, mediaErr := afero.ReadFile(fileSystem, utility.ImageName)
	if mediaErr != nil {
		return mediaErr
	}
	checksum, checksumOpenErr := afero.ReadFile(fileSystem, checksumName)
	if checksumOpenErr != nil {
		return checksumOpenErr
	}

	return ValidateHashes(ctx, utility.ImageName, media, checksum)
}

func DownloadFile(ctx context.Context, fileSystem afero.Fs, fileName string, url string) error {

	ctx, span := telemetry.GetTracer().Start(ctx, "Download")
	span.AddEvent(fmt.Sprintf("downloading: %s", fileName))
	defer span.End()
	media, mediaErr := fileSystem.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if mediaErr != nil {
		return mediaErr
	}
	defer utility.WrappedClose(media)

	mediaResponse, mediaDownloadErr := otelhttp.Get(ctx, url)
	if mediaDownloadErr != nil {
		return mediaDownloadErr
	}
	defer utility.WrappedClose(mediaResponse.Body)
	if mediaResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("received non 200 status code: %d", mediaResponse.StatusCode)
	}

	_, copyErr := io.Copy(media, mediaResponse.Body)
	if copyErr != nil {
		return copyErr
	}
	return nil
}

func ValidateHashes(ctx context.Context, fileName string, mediaBytes []byte, checksumBytes []byte) error {
	_, span := telemetry.GetTracer().Start(ctx, "hash validate")
	defer span.End()
	hash := sha256.New()
	hash.Write(mediaBytes)
	mediaHash := []byte(hex.EncodeToString(hash.Sum(nil)))
	checksums, parseErr := extractChecksum(checksumBytes)
	if parseErr != nil {
		return parseErr
	}
	if !bytes.Equal(mediaHash, checksums[fileName]) {
		return errors.New("checksums do not match")
	}
	return nil
}

func extractChecksum(fileBytes []byte) (map[string][]byte, error) {
	sums := make(map[string][]byte)
	split := bytes.Split(fileBytes, []byte("\n"))
	for _, line := range split {
		// break on last line since it's empty
		if len(line) == 0 {
			break
		}
		lineSplit := bytes.Split(line, []byte(" *"))
		if len(lineSplit) != 2 {
			return sums, errors.New("length mismatch check file format")
		}
		sums[string(lineSplit[1])] = lineSplit[0]
	}
	return sums, nil
}
