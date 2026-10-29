package RequestHandlers

import (
	"errors"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"net/http"
	"strconv"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/iconlib"
	"github.com/labstack/echo/v4"
)

// HandleSearchIconLibrary handles GET /shared_api/v1/icons/search?q=&category=&limit=
// The catalogue is read-only reference data, so no authentication is required.
func HandleSearchIconLibrary(c echo.Context) error {
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	icons, err := iconlib.Search(c.Request().Context(), ApiTypes.SharedDBHandle,
		c.QueryParam("q"), c.QueryParam("category"), limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ApiTypes.JimoResponse{
			Status: false, ErrorMsg: "icon search failed", Loc: "SHD_ICL_020"})
	}
	return c.JSON(http.StatusOK, ApiTypes.JimoResponse{
		Status: true, ResultType: "json_array", NumRecords: len(icons), Results: icons, Loc: "SHD_ICL_024"})
}

// HandleGetIconLibrarySVG handles GET /shared_api/v1/icons/by-name/:name/svg.
// The body is cacheable; the ETag is the icon's version.
func HandleGetIconLibrarySVG(c echo.Context) error {
	svg, version, err := iconlib.GetSVG(c.Request().Context(), ApiTypes.SharedDBHandle, c.Param("name"))
	if errors.Is(err, iconlib.ErrNotFound) {
		return c.JSON(http.StatusNotFound, ApiTypes.JimoResponse{
			Status: false, ErrorMsg: "icon not found", Loc: "SHD_ICL_034"})
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ApiTypes.JimoResponse{
			Status: false, ErrorMsg: "icon lookup failed", Loc: "SHD_ICL_038"})
	}
	etag := `"` + strconv.Itoa(version) + `"`
	h := c.Response().Header()
	h.Set("ETag", etag)
	h.Set("Cache-Control", "public, max-age=3600")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	if c.Request().Header.Get("If-None-Match") == etag {
		return c.NoContent(http.StatusNotModified)
	}
	return c.Blob(http.StatusOK, "image/svg+xml", []byte(svg))
}

// HandleAddIconLibraryIcon handles POST /shared_api/v1/icons/library (JSON body,
// see iconlib.CustomIcon). Only SVG is accepted; the markup is sanitised.
func HandleAddIconLibraryIcon(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "SHD_ICL_060")
	defer rc.Close()
	user := rc.IsAuthenticated()
	if user == nil {
		return c.JSON(http.StatusUnauthorized, ApiTypes.JimoResponse{
			Status: false, ErrorMsg: "Authentication required", Loc: "SHD_ICL_064"})
	}
	var ic iconlib.CustomIcon
	if err := c.Bind(&ic); err != nil {
		return c.JSON(http.StatusBadRequest, ApiTypes.JimoResponse{
			Status: false, ErrorMsg: "invalid request body", Loc: "SHD_ICL_070"})
	}
	err := iconlib.AddCustom(c.Request().Context(), ApiTypes.SharedDBHandle, ic, user.Email)
	switch {
	case errors.Is(err, iconlib.ErrExists):
		return c.JSON(http.StatusConflict, ApiTypes.JimoResponse{
			Status: false, ErrorMsg: err.Error(), Loc: "SHD_ICL_076"})
	case err != nil:
		return c.JSON(http.StatusBadRequest, ApiTypes.JimoResponse{
			Status: false, ErrorMsg: err.Error(), Loc: "SHD_ICL_079"})
	}
	return c.JSON(http.StatusOK, ApiTypes.JimoResponse{Status: true, Loc: "SHD_ICL_082"})
}
