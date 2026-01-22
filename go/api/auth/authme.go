package auth

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	"github.com/labstack/echo/v4"
)

func HandleAuthMe(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "SHD_ATM_017")
	reqID := rc.ReqID()
	status_code, resp := HandleAuthMeBase(rc, reqID)
	c.JSON(status_code, resp)
	return nil
}

func HandleAuthMeBase(
	rc ApiTypes.RequestContext,
	reqID string) (int, ApiTypes.JimoResponse) {
	// This function is called by the route "/auth/me" (in Shared/go/api/auth/router.go)
	logger := rc.GetLogger()
	user_info := rc.IsAuthenticated()
	if user_info == nil {
		error_msg := "auth failed"
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_UserNotAuthed,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_AuthMe,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_ATM_048"})

		var resp = ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "user not logged in",
			Loc:      "SHD_AME_029",
		}

		logger.Warn("user not logged in")
		return ApiTypes.CustomHttpStatus_NotLoggedIn, resp
	}

	user_info_str, _ := json.Marshal(user_info)
	base_url := os.Getenv("APP_DOMAIN_NAME")
	var resp = ApiTypes.JimoResponse{
		Status:     true,
		ErrorMsg:   "",
		Results:    string(user_info_str),
		ResultType: "json",
		BaseURL:    base_url,
		Loc:        "SHD_AME_047",
	}

	logger.Info("AuthMe success",
		"email", user_info.Email,
		"status", user_info.UserStatus,
		"user_id", user_info.UserId,
		"is_admin", user_info.Admin)
	return http.StatusOK, resp
}

func HandleABC(c echo.Context) error {
	return nil
}
