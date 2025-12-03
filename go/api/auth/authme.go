package auth

import (
	"fmt"
	"net/http"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	middleware "github.com/chendingplano/shared/go/auth-middleware"
	"github.com/labstack/echo/v4"
)

func HandleAuthMe(c echo.Context) error {
	// This function is called by the route "/auth/me" (in Shared/go/api/auth/router.go)
	user_name, err := middleware.IsAuthenticated(c, "SHD_AME_014")
	if err != nil {
		error_msg := fmt.Sprintf("auth failed, err:%v", err)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: 		ApiTypes.ActivityName_Auth,
			ActivityType: 		ApiTypes.ActivityType_UserNotAuthed,
			AppName: 			ApiTypes.AppName_Auth,
			ModuleName: 		ApiTypes.ModuleName_AuthMe,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_ATM_075"})

		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "User not authenticated",
			"loc": "(SHD_AME_016)"})
	}

	msg := fmt.Sprintf("user is logged in:%s", user_name)
	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: 		ApiTypes.ActivityName_Auth,
		ActivityType: 		ApiTypes.ActivityType_UserIsLoggedIn,
		AppName: 			ApiTypes.AppName_Auth,
		ModuleName: 		ApiTypes.ModuleName_AuthMe,
		ActivityMsg: 		&msg,
		CallerLoc: 			"SHD_ATM_089"})

	return c.JSON(http.StatusOK, echo.Map{
		    "authenticated": true,
        	"user_name": user_name,
			"loc": "(SHD_AME_023)",
    		})
}


func HandleABC(c echo.Context) error {
	return nil
}