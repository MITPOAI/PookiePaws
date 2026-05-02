package opencut

import "errors"

type ExportRequest struct {
	EditPlanPath string `json:"edit_plan_path"`
	OutputDir    string `json:"output_dir"`
}

type ImportRequest struct {
	ProjectPath string `json:"project_path"`
	OutputDir   string `json:"output_dir"`
}

type BridgeResult struct {
	Supported bool   `json:"supported"`
	Message   string `json:"message"`
	Path      string `json:"path,omitempty"`
}

func ExportProject(req ExportRequest) (BridgeResult, error) {
	return BridgeResult{
		Supported: false,
		Message:   "OpenCut is optional. pookiepaws will add an export bridge only after OpenCut exposes a stable project/render interface suitable for automation.",
	}, errors.New("OpenCut export is not enabled in the MVP")
}

func ImportProject(req ImportRequest) (BridgeResult, error) {
	return BridgeResult{
		Supported: false,
		Message:   "OpenCut import is optional and not enabled until the project format and render workflow are stable.",
	}, errors.New("OpenCut import is not enabled in the MVP")
}

func SupportReport() string {
	return "OpenCut is a promising open-source editor, but it is a full app stack rather than the core pookiepaws renderer. FFmpeg remains the default renderer; OpenCut is reserved for a future bridge."
}
