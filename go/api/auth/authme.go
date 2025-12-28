package auth

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/RequestHandlers"
	"github.com/labstack/echo/v4"
	"github.com/pocketbase/pocketbase/core"
)

func HandleAuthMe(c echo.Context) error {
	rc := RequestHandlers.NewFromEcho(c)
	reqID := rc.ReqID()
	status_code, resp := HandleAuthMeBase(rc, reqID)
	c.JSON(status_code, resp)
	return nil
}

func HandleAuthMePocket(e *core.RequestEvent) error {
	log.Printf("AuthMe (SHD_ATH_025)")
	rc := RequestHandlers.NewFromPocket(e)
	reqID := rc.ReqID()
	status_code, resp := HandleAuthMeBase(rc, reqID)
	e.JSON(status_code, resp)
	return nil
}

func HandleAuthMeBase(
	rc RequestHandlers.RequestContext,
	reqID string) (int, ApiTypes.JimoResponse) {
	// This function is called by the route "/auth/me" (in Shared/go/api/auth/router.go)
	user_info, err := rc.IsAuthenticated(reqID, "SHD_AME_014")
	if err != nil {
		error_msg := fmt.Sprintf("auth failed, err:%v", err)
		/*
			sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
				ActivityName: ApiTypes.ActivityName_Auth,
				ActivityType: ApiTypes.ActivityType_UserNotAuthed,
				AppName:      ApiTypes.AppName_Auth,
				ModuleName:   ApiTypes.ModuleName_AuthMe,
				ActivityMsg:  &error_msg,
				CallerLoc:    "SHD_ATM_075"})
		*/

		var resp = ApiTypes.JimoResponse{
			Status:   false,
			ErrorMsg: "user not logged in",
			Loc:      "SHD_AME_029",
		}

		log.Printf("AuthMe, user not logged in:%s (SHD_AME_034)", error_msg)
		return ApiTypes.CustomHttpStatus_NotLoggedIn, resp
	}

	/*
		msg := fmt.Sprintf("user is logged in:%s", user_info["user_name"])
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_UserIsLoggedIn,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_AuthMe,
			ActivityMsg:  &msg,
			CallerLoc:    "SHD_AME_042"})
	*/

	user_info_str, _ := json.Marshal(user_info)
	var resp = ApiTypes.JimoResponse{
		Status:     true,
		ErrorMsg:   "",
		Results:    string(user_info_str),
		ResultType: "json",
		Loc:        "SHD_AME_047",
	}

	log.Printf("AuthMe success, email:%s (SHD_AME_033)", user_info.Email)
	return http.StatusOK, resp
}

func HandleABC(c echo.Context) error {
	return nil
}
