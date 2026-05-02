package capcut

import "errors"

type ExportRequest struct {
	EditPlanPath string `json:"edit_plan_path"`
	OutputDir    string `json:"output_dir"`
}

type ExportResult struct {
	Supported bool   `json:"supported"`
	Message   string `json:"message"`
	Path      string `json:"path,omitempty"`
}

func ExportDraft(req ExportRequest) (ExportResult, error) {
	return ExportResult{
		Supported: false,
		Message:   "CapCut/Jianying draft export is intentionally optional. Use the FFmpeg renderer as the reliable path until a tested adapter is added.",
	}, errors.New("CapCut draft export is not enabled in the MVP")
}

func SupportReport() string {
	return "CapCut draft automation has active open-source options, but draft schemas are version-sensitive. pookiepaws keeps FFmpeg as the core renderer and treats CapCut export as an optional adapter."
}
