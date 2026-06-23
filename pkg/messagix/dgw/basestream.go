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

package dgw

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/coder/websocket"
	"github.com/rs/zerolog"
	"github.com/tidwall/gjson"
	"go.mau.fi/util/exsync"
	"go.mau.fi/util/ptr"
)

type Stream interface {
	receiveFrame(context.Context, *DataFrame) error
	receiveEstablish(frame *EstablishStreamFrame)
	receiveAck(id uint16) bool
	close()
}

var (
	_ Stream = (*PersistentStream)(nil)
	_ Stream = (*OneOffStream)(nil)
)

type baseStream struct {
	log  *zerolog.Logger
	conn *websocket.Conn
	id   StreamID

	established  *exsync.Event
	establishErr atomic.Pointer[error]
}

func newBaseStream(conn *websocket.Conn, id StreamID, log *zerolog.Logger) baseStream {
	return baseStream{
		log:         log,
		conn:        conn,
		id:          id,
		established: exsync.NewEvent(),
	}
}

func receiveEstablish(f *EstablishStreamFrame, log *zerolog.Logger) error {
	if res := gjson.GetBytes(f.RawParameters, "code"); !res.Exists() {
		log.Warn().
			Uint16("stream_id", uint16(f.StreamID)).
			RawJSON("raw_params", f.RawParameters).
			Msg("Encountered establish stream response without code field")
		return fmt.Errorf("no response status code")
	} else if res.Num != 200 {
		return fmt.Errorf("code %d", int(res.Num))
	}
	log.Debug().
		Uint16("stream_id", uint16(f.StreamID)).
		RawJSON("parameters", f.RawParameters).
		Msg("Stream established")
	return nil
}

func (s *baseStream) receiveEstablish(f *EstablishStreamFrame) {
	if err := receiveEstablish(f, s.log); err != nil {
		s.establishErr.Store(ptr.Ptr(err))
	}
	s.established.Set()
}
