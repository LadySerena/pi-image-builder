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
	"crypto/md5" //nolint:gosec
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"
)

func DownloadAndVerifyMedia(fileSystem afero.Fs, forceOverwrite bool) error {

	client := http.Client{
		Timeout: time.Minute * 5,
	}

	mediaName := "ArchLinuxARM-rpi-aarch64-latest.tar.gz"
	checksumName := fmt.Sprintf("%s.md5", mediaName)

	_, mediaStatErr := fileSystem.Stat(mediaName)
	_, checksumStatErr := fileSystem.Stat(checksumName)

	if needsToDownload(forceOverwrite, mediaStatErr, checksumStatErr) {
		group := new(errgroup.Group)
		group.Go(func() error {
			return DownloadFile(&client, fileSystem, mediaName, fmt.Sprintf("http://os.archlinuxarm.org/os/%s", mediaName))
		})
		group.Go(func() error {
			return DownloadFile(&client, fileSystem, checksumName, fmt.Sprintf("http://os.archlinuxarm.org/os/%s", checksumName))
		})
		if waitErr := group.Wait(); waitErr != nil {
			return waitErr
		}
	}

	media, mediaErr := afero.ReadFile(fileSystem, mediaName)
	if mediaErr != nil {
		return mediaErr
	}
	checksum, checksumOpenErr := afero.ReadFile(fileSystem, checksumName)
	if checksumOpenErr != nil {
		return checksumOpenErr
	}

	return ValidateHashes(media, checksum)
}

func needsToDownload(force bool, mediaErr error, checksumErr error) bool {
	return force || (errors.Is(mediaErr, fs.ErrNotExist) || errors.Is(checksumErr, fs.ErrNotExist))
}

func DownloadFile(client *http.Client, fileSystem afero.Fs, fileName string, url string) error {
	media, mediaErr := fileSystem.Create(fileName)
	if mediaErr != nil {
		return mediaErr
	}
	defer wrappedClose(media)

	mediaResponse, mediaDownloadErr := client.Get(url)
	if mediaDownloadErr != nil {
		return mediaDownloadErr
	}
	defer wrappedClose(mediaResponse.Body)

	_, copyErr := io.Copy(media, mediaResponse.Body)
	if copyErr != nil {
		return copyErr
	}
	return nil
}

func ValidateHashes(mediaBytes []byte, md5fileBytes []byte) error {
	hash := md5.New() //nolint:gosec
	if _, hashErr := hash.Write(mediaBytes); hashErr != nil {
		return hashErr
	}
	sum := hash.Sum(nil)
	mediaHash := hex.EncodeToString(sum)
	checksum, extractErr := extractChecksum(md5fileBytes)
	if extractErr != nil {
		return extractErr
	}
	if mediaHash != checksum {
		return errors.New("checksums do not match")
	}
	return nil
}

func extractChecksum(fileBytes []byte) (string, error) {
	split := bytes.Split(fileBytes, []byte(" "))
	if len(split) != 3 {
		return "", errors.New("length mismatch check file format")
	}
	return string(split[0]), nil
}

func wrappedClose(closer io.Closer) {
	if err := closer.Close(); err != nil {
		log.Fatalf("could not close closer properly: %v", err)
	}
}
