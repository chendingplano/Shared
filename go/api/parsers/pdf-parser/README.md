# PDF Parser Service (`pdf-parser`)

This package implements a polling PDF parser service for `kb.inputs` using PaddleOCR.

## Behavior

- Polls `kb.inputs` from `ApiTypes.ProjectDBHandle`.
- Picks records where `type='pdf'` and `status` does not already contain operation `parse`.
- Uses Python + PaddleOCR script to parse the PDF.
- Writes OCR result JSON to repo directory as `ocr_rslt_<record_id>.json`.
- Picks the least-used repo directory (smallest total bytes).
- Copies source PDF to selected repo and backup destination.
- Appends parse status (`success` / `fail`) into `status` JSON.
- Updates `result_filename` in `kb.inputs`.

## Minimal Usage

```go
cfg := pdfparser.Config{
    StagingDir: "/data/staging",
    RepoDirs:   []string{"/data/repo1", "/data/repo2"},
    BackupDir:  "/data/backup",

    // Optional overrides
    PaddleOCRScript: "/Users/cding/Workspace/ThirdParty/paddleocr/parse_pdf.py",
    PythonBin:       "python3",
}

svc, err := pdfparser.NewService(cfg)
if err != nil {
    return err
}

logger := loggerutil.CreateDefaultLogger()
defer logger.Close()

ctx, cancel := context.WithCancel(context.Background())
defer cancel()

if err := svc.Run(ctx, logger); err != nil {
    return err
}
```
