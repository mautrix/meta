// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2024 Tulir Asokan
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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"strings"

	"github.com/beeper/libserv/pkg/requestlog"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/database"
	"go.mau.fi/mautrix-meta/messagix/cookies"
)

type provisioningContextKey int

const (
	provisioningUserKey provisioningContextKey = iota
)

type ProvisioningAPI struct {
	bridge *MetaBridge
	log    zerolog.Logger
}

func (prov *ProvisioningAPI) Init() {
	prov.log.Debug().Str("prefix", prov.bridge.Config.Bridge.Provisioning.Prefix).Msg("Enabling provisioning API")
	r := prov.bridge.AS.Router.PathPrefix(prov.bridge.Config.Bridge.Provisioning.Prefix).Subrouter()
	r.Use(hlog.NewHandler(prov.log))
	r.Use(requestlog.AccessLogger(true))
	r.Use(prov.AuthMiddleware)
	r.HandleFunc("/v1/login", prov.Login).Methods(http.MethodPost)
	r.HandleFunc("/v1/logout", prov.Logout).Methods(http.MethodPost)

	if prov.bridge.Config.Bridge.Provisioning.DebugEndpoints {
		prov.log.Debug().Msg("Enabling debug API at /debug")
		r := prov.bridge.AS.Router.PathPrefix("/debug").Subrouter()
		r.Use(prov.AuthMiddleware)
		r.PathPrefix("/pprof").Handler(http.DefaultServeMux)
	}
}

func jsonResponse(w http.ResponseWriter, status int, response any) {
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

func (prov *ProvisioningAPI) AuthMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if auth != prov.bridge.Config.Bridge.Provisioning.SharedSecret {
			zerolog.Ctx(r.Context()).Warn().Msg("Authentication token does not match shared secret")
			jsonResponse(w, http.StatusForbidden, &mautrix.RespError{
				Err:     "Authentication token does not match shared secret",
				ErrCode: mautrix.MForbidden.ErrCode,
			})
			return
		}
		userID := r.URL.Query().Get("user_id")
		user := prov.bridge.GetUserByMXID(id.UserID(userID))
		h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), provisioningUserKey, user)))
	})
}

type Error struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	ErrCode string `json:"errcode"`
}

type Response struct {
	Success bool   `json:"success"`
	Status  string `json:"status"`
}

func (prov *ProvisioningAPI) Login(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(provisioningUserKey).(*User)
	log := prov.log.With().
		Str("action", "login").
		Str("user_id", user.MXID.String()).
		Logger()
	ctx := log.WithContext(r.Context())
	var newCookies cookies.Cookies
	newCookies.Platform = database.MessagixPlatform
	err := json.NewDecoder(r.Body).Decode(&newCookies)
	if err != nil {
		jsonResponse(w, http.StatusBadRequest, Error{ErrCode: mautrix.MBadJSON.ErrCode, Error: err.Error()})
		return
	}
	missingRequiredCookies := newCookies.GetMissingCookieNames()
	if len(missingRequiredCookies) > 0 {
		log.Debug().Any("missing_cookies", missingRequiredCookies).Msg("Missing cookies in login request")
		jsonResponse(w, http.StatusBadRequest, Error{ErrCode: mautrix.MBadJSON.ErrCode, Error: fmt.Sprintf("Missing cookies: %v", missingRequiredCookies)})
		return
	}
	err = user.Login(ctx, &newCookies)
	if err != nil {
		log.Err(err).Msg("Failed to log in")
		jsonResponse(w, http.StatusInternalServerError, Error{ErrCode: "M_UNKNOWN", Error: "Internal error logging in"})
	} else {
		jsonResponse(w, http.StatusOK, Response{
			Success: true,
			Status:  "logged_in",
		})
	}
}

func (prov *ProvisioningAPI) Logout(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(provisioningUserKey).(*User)
	user.DeleteSession()
	jsonResponse(w, http.StatusOK, Response{
		Success: true,
		Status:  "logged_out",
	})
}
