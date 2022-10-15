/*
 * Copyright (c) 2022 Serena Tiede
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

package partition

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPartedCommand(t *testing.T) {
	cases := []struct {
		input    []string
		expected []string
	}{
		{
			input:    []string{"/dev/loop8", "mktable", "msdos"},
			expected: []string{"parted", "-s", "/dev/loop8", "mktable", "msdos"},
		},
	}
	for index, tt := range cases {
		// test fails but is the same 🤔
		actual := partedCommand(tt.input[0], tt.input[1], tt.input[2])
		if !reflect.DeepEqual(actual.Args, tt.expected) {
			t.Errorf("partedCommand(%d): expected %v, actual %v", index, tt.expected, actual.Args)
		}
	}
}

func TestGetLogicalVolumeSizes(t *testing.T) {
	foo := VolumeGroupEntry{
		Name:        "rootvg",
		PvCount:     "1",
		LvCount:     "0",
		SnapCount:   "0",
		VGAttribute: "wz--n-",
		VGSize:      "31394365440B",
		VGFree:      "31394365440B",
	}
	expected := SlicedVolumeGroup{
		RootVolumeSize: 10737418240,
		CSIVolumeSize:  20120076288,
	}
	rootSize, csiSize, _, err := GetLogicalVolumeSizes(foo)
	assert.NoError(t, err)
	assert.Equal(t, expected.RootVolumeSize, rootSize)
	assert.Equal(t, expected.CSIVolumeSize, csiSize)
}
