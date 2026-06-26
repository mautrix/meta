// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package thrift

import (
	"bytes"
	"context"

	"github.com/apache/thrift/lib/go/thrift"
)

//go:generate ./generate.sh

func Unmarshal(data []byte, into thrift.TStruct) error {
	buf := thrift.NewTMemoryBuffer()
	buf.Buffer = bytes.NewBuffer(data)
	return into.Read(context.Background(), thrift.NewTCompactProtocolConf(buf, nil))
}

func Marshal(data thrift.TStruct) ([]byte, error) {
	buf := thrift.NewTMemoryBuffer()
	err := data.Write(context.Background(), thrift.NewTCompactProtocolConf(buf, nil))
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
