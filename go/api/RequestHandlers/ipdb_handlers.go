package RequestHandlers

import (
	"net/http"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/ipdb"
	"github.com/labstack/echo/v4"
)

// HandleIPLookup handles GET /shared_api/v1/ipdb/lookup?ip=<address>
//
// Returns ASN, country, and continent data for the requested IP address.
// Authentication is required.
func HandleIPLookup(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "SHD_IPD_410")
	defer rc.Close()
	log := rc.GetLogger()

	userInfo := rc.IsAuthenticated()
	if userInfo == nil {
		return c.JSON(http.StatusUnauthorized, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Authentication required",
			Loc:      "SHD_IPD_420",
		})
	}

	ip := c.QueryParam("ip")
	if ip == "" {
		return c.JSON(http.StatusBadRequest, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Query parameter 'ip' is required",
			Loc:      "SHD_IPD_428",
		})
	}

	if err := ipdb.ValidateIP(ip); err != nil {
		return c.JSON(http.StatusBadRequest, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Invalid IP address",
			Loc:      "SHD_IPD_436",
		})
	}

	rec, err := ipdb.LookupIP(log, ip)
	if err != nil {
		log.Error("ipdb: lookup failed", "error", err, "ip", ip)
		return c.JSON(http.StatusInternalServerError, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "IP lookup failed: " + err.Error(),
			Loc:      "SHD_IPD_445",
		})
	}

	return c.JSON(http.StatusOK, ApiTypes.JimoResponse{
		Status:     true,
		ResultType: "json",
		NumRecords: 1,
		Results:    rec,
		Loc:        "SHD_IPD_453",
	})
}

// HandleIPSyncStatus handles GET /shared_api/v1/ipdb/sync/status
//
// Returns the most recent database synchronisation record.
// Authentication is required.
func HandleIPSyncStatus(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "SHD_IPD_462")
	defer rc.Close()
	log := rc.GetLogger()

	userInfo := rc.IsAuthenticated()
	if userInfo == nil {
		return c.JSON(http.StatusUnauthorized, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Authentication required",
			Loc:      "SHD_IPD_471",
		})
	}

	status, err := ipdb.GetLastSyncStatus(ApiTypes.ProjectDBHandle)
	if err != nil {
		log.Error("ipdb: failed to read sync status", "error", err)
		return c.JSON(http.StatusInternalServerError, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Failed to read sync status",
			Loc:      "SHD_IPD_481",
		})
	}

	return c.JSON(http.StatusOK, ApiTypes.JimoResponse{
		Status:     true,
		ResultType: "json",
		NumRecords: 1,
		Results:    status,
		Loc:        "SHD_IPD_489",
	})
}

// HandleIPSyncTrigger handles POST /shared_api/v1/ipdb/sync/trigger
//
// Triggers an immediate MMDB download and cache purge in the background.
// Admin access is required.
func HandleIPSyncTrigger(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "SHD_IPD_498")
	defer rc.Close()
	log := rc.GetLogger()

	userInfo := rc.IsAuthenticated()
	if userInfo == nil {
		return c.JSON(http.StatusUnauthorized, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Authentication required",
			Loc:      "SHD_IPD_507",
		})
	}

	if !userInfo.Admin {
		return c.JSON(http.StatusForbidden, ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "Admin access required",
			Loc:      "SHD_IPD_515",
		})
	}

	go func() {
		if err := ipdb.Sync(log); err != nil {
			log.Warn("ipdb: manual sync failed", "error", err)
		}
	}()

	return c.JSON(http.StatusAccepted, ApiTypes.JimoResponse{
		Status:  true,
		Results: map[string]string{"message": "Sync triggered"},
		Loc:     "SHD_IPD_526",
	})
}
