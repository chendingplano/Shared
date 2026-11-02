package RequestHandlers

import (
	"errors"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/emojilib"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
	"net/http"
	"strconv"
)

var emojiLogger = loggerutil.CreateDefaultLogger("20261102-437")

func emojiFailure(c echo.Context, err error) error {
	if errors.Is(err, emojilib.ErrNotFound) {
		return c.JSON(http.StatusNotFound, ApiTypes.JimoResponse{Status: false, ErrorMsg: "emoji not found"})
	}
	if errors.Is(err, emojilib.ErrBM25Unavailable) {
		emojiLogger.Warn("emoji BM25 unavailable")
		return c.JSON(http.StatusServiceUnavailable, echo.Map{"status": false, "error_msg": "emoji search unavailable", "reason": "bm25_unavailable"})
	}
	emojiLogger.Error("emoji library request failed", "error", err)
	return c.JSON(http.StatusInternalServerError, ApiTypes.JimoResponse{Status: false, ErrorMsg: "emoji library unavailable"})
}
func HandleSearchEmojiLibrary(c echo.Context) error {
	if len(c.QueryParam("q")) > 512 {
		return c.JSON(http.StatusBadRequest, ApiTypes.JimoResponse{Status: false, ErrorMsg: "query too long"})
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	page, err := emojilib.Search(c.Request().Context(), ApiTypes.SharedDBHandle, c.QueryParam("q"), c.QueryParam("category"), limit, offset)
	if err != nil {
		return emojiFailure(c, err)
	}
	return c.JSON(http.StatusOK, ApiTypes.JimoResponse{Status: true, ResultType: "json_object", NumRecords: len(page.Items), Results: page})
}
func HandleGetEmoji(c echo.Context) error {
	e, err := emojilib.Get(c.Request().Context(), ApiTypes.SharedDBHandle, c.Param("id"))
	if err != nil {
		return emojiFailure(c, err)
	}
	return c.JSON(http.StatusOK, ApiTypes.JimoResponse{Status: true, Results: e})
}
func HandleEmojiCategories(c echo.Context) error {
	a, err := emojilib.Categories(c.Request().Context(), ApiTypes.SharedDBHandle)
	if err != nil {
		return emojiFailure(c, err)
	}
	return c.JSON(http.StatusOK, ApiTypes.JimoResponse{Status: true, Results: a, NumRecords: len(a)})
}

func HandleEmojiReadiness(c echo.Context) error {
	if err := emojilib.Ready(c.Request().Context(), ApiTypes.SharedDBHandle); err != nil {
		return emojiFailure(c, err)
	}
	return c.JSON(http.StatusOK, echo.Map{"status": true, "results": echo.Map{"bm25": true}})
}
func HandleEmojiSVG(c echo.Context) error {
	svg, hash, err := emojilib.SVG(c.Request().Context(), ApiTypes.SharedDBHandle, c.Param("id"))
	if err != nil {
		return emojiFailure(c, err)
	}
	tag := `"` + hash + `"`
	h := c.Response().Header()
	h.Set("ETag", tag)
	h.Set("Cache-Control", "public, max-age=3600")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'none'; sandbox")
	if c.Request().Header.Get("If-None-Match") == tag {
		return c.NoContent(http.StatusNotModified)
	}
	return c.Blob(http.StatusOK, "image/svg+xml", []byte(svg))
}

func HandleEmojiNotice(c echo.Context) error {
	name := c.Param("name")
	switch name {
	case "NOTICE.txt", "TWEMOJI-CODE-MIT.txt", "TWEMOJI-GRAPHICS-CC-BY-4.0.txt", "UNICODE-CLDR.txt":
	default:
		return c.NoContent(http.StatusNotFound)
	}
	data, err := emojilib.Notices.ReadFile("licenses/" + name)
	if err != nil {
		return emojiFailure(c, err)
	}
	return c.Blob(http.StatusOK, "text/plain; charset=utf-8", data)
}
