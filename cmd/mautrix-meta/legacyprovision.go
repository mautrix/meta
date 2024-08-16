package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/connector"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

func jsonResponse(w http.ResponseWriter, status int, response any) {
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
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

func modeToLoginFlowID(mode types.Platform) string {
	switch mode {
	case types.Facebook, types.FacebookTor:
		return connector.FlowIDFacebookCookies
	case types.Messenger:
		return connector.FlowIDMessengerCookies
	case types.Instagram:
		return connector.FlowIDInstagramCookies
	default:
		return ""
	}
}

func legacyProvLogin(w http.ResponseWriter, r *http.Request) {
	user := m.Matrix.Provisioning.GetUser(r)
	ctx := r.Context()
	var newCookies map[string]string
	err := json.NewDecoder(r.Body).Decode(&newCookies)
	if err != nil {
		jsonResponse(w, http.StatusBadRequest, Error{ErrCode: mautrix.MBadJSON.ErrCode, Error: err.Error()})
		return
	}
	lp, err := c.CreateLogin(ctx, user, modeToLoginFlowID(c.Config.Mode))
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to create login")
		jsonResponse(w, http.StatusInternalServerError, Error{ErrCode: "M_UNKNOWN", Error: "Internal error creating login"})
	} else if firstStep, err := lp.Start(ctx); err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to start login")
		jsonResponse(w, http.StatusInternalServerError, Error{ErrCode: "M_UNKNOWN", Error: "Internal error starting login"})
	} else if firstStep.StepID != connector.LoginStepIDCookies {
		jsonResponse(w, http.StatusInternalServerError, Error{ErrCode: "M_UNKNOWN", Error: "Unexpected login step"})
	} else if finalStep, err := lp.(bridgev2.LoginProcessCookies).SubmitCookies(ctx, newCookies); err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to log in")
		var respErr bridgev2.RespError
		if errors.As(err, &respErr) {
			jsonResponse(w, respErr.StatusCode, &respErr)
		} else {
			jsonResponse(w, http.StatusInternalServerError, Error{ErrCode: "M_UNKNOWN", Error: "Internal error logging in"})
		}
	} else if finalStep.StepID != connector.LoginStepIDComplete {
		jsonResponse(w, http.StatusInternalServerError, Error{ErrCode: "M_UNKNOWN", Error: "Unexpected login step"})
	} else {
		jsonResponse(w, http.StatusOK, Response{
			Success: true,
			Status:  "logged_in",
		})
	}
}

func legacyProvLogout(w http.ResponseWriter, r *http.Request) {
	user := m.Matrix.Provisioning.GetUser(r)
	logins := user.GetCachedUserLogins()
	for _, login := range logins {
		login.Logout(r.Context())
	}
	jsonResponse(w, http.StatusOK, Response{
		Success: true,
		Status:  "logged_out",
	})
}
