package types

type BootLoaderConfig struct {
	BtCutoffIndex              int   `json:"btCutoffIndex,omitempty"`
	DeferBootloads             bool  `json:"deferBootloads,omitempty"`
	EarlyRequireLazy           bool  `json:"earlyRequireLazy,omitempty"`
	FastPathForAlreadyRequired bool  `json:"fastPathForAlreadyRequired,omitempty"`
	HypStep4                   bool  `json:"hypStep4,omitempty"`
	JsRetries                  []int `json:"jsRetries,omitempty"`
	JsRetryAbortNum            int   `json:"jsRetryAbortNum,omitempty"`
	JsRetryAbortTime           int   `json:"jsRetryAbortTime,omitempty"`
	PhdOn                      bool  `json:"phdOn,omitempty"`
	SilentDups                 bool  `json:"silentDups,omitempty"`
	Timeout                    int   `json:"timeout,omitempty"`
	TranslationRetries         []int `json:"translationRetries,omitempty"`
	TranslationRetryAbortNum   int   `json:"translationRetryAbortNum,omitempty"`
	TranslationRetryAbortTime  int   `json:"translationRetryAbortTime,omitempty"`
}

type Bootloader_HandlePayload struct {
	Consistency Consistency            `json:"consistency,omitempty"`
	RsrcMap     map[string]RsrcDetails `json:"rsrcMap,omitempty"`
	CsrUpgrade  string                 `json:"csrUpgrade,omitempty"`
}

type Consistency struct {
	Rev int64 `json:"rev,omitempty"`
}

type RsrcDetails struct {
	Type string `json:"type,omitempty"`
	Src  string `json:"src,omitempty"`
	C    int64  `json:"c,omitempty"`
	Tsrc string `json:"tsrc,omitempty"`
	P    string `json:"p,omitempty"`
	M    string `json:"m,omitempty"`
}
