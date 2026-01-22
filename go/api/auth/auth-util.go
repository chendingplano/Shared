package auth

import (
	"fmt"
	"os"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/sysdatastores"
)

func GetRedirectURL(
	rc ApiTypes.RequestContext,
	email string,
	is_admin bool,
	domain_name_only bool) string {
	logger := rc.GetLogger()
	home_domain := os.Getenv("APP_DOMAIN_NAME")
	if home_domain == "" {
		error_msg := fmt.Sprintf("missing APP_DOMAIN_NAME env var, email:%s, default to:%s",
			email, home_domain)
		logger.Error("missing APP_DOMAIN_NAME")

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			ActivityName: ApiTypes.ActivityName_Auth,
			ActivityType: ApiTypes.ActivityType_MissHomeURL,
			AppName:      ApiTypes.AppName_Auth,
			ModuleName:   ApiTypes.ModuleName_EmailAuth,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_ATL_025"})
	}

	if domain_name_only {
		return home_domain
	}

	var redirect_url string = fmt.Sprintf("%s/", home_domain)
	if is_admin {
		default_admin_app := os.Getenv("APP_DEFAULT_ADMIN_APP")
		if default_admin_app != "" {
			redirect_url += default_admin_app
		} else {
			redirect_url += "admin/dashboard"
			error_msg := fmt.Sprintf("missing APP_DEFAULT_ADMIN_APP env var, email:%s, default to:%s",
				email, redirect_url)
			logger.Error("missing APP_DEFAULT_ADMIN_APP")

			sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
				ActivityName: ApiTypes.ActivityName_Auth,
				ActivityType: ApiTypes.ActivityType_MissHomeURL,
				AppName:      ApiTypes.AppName_Auth,
				ModuleName:   ApiTypes.ModuleName_EmailAuth,
				ActivityMsg:  &error_msg,
				CallerLoc:    "SHD_ATL_046"})
		}
	} else {
		default_app := os.Getenv("APP_DEFAULT_APP")
		if default_app != "" {
			redirect_url += default_app
		} else {
			redirect_url += "dashboard"
			error_msg := fmt.Sprintf("missing APP_DEFAULT_APP env var, email:%s, default to:%s",
				email, redirect_url)
			logger.Error("missing APP_DEFAULT_APP")

			sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
				ActivityName: ApiTypes.ActivityName_Auth,
				ActivityType: ApiTypes.ActivityType_MissHomeURL,
				AppName:      ApiTypes.AppName_Auth,
				ModuleName:   ApiTypes.ModuleName_EmailAuth,
				ActivityMsg:  &error_msg,
				CallerLoc:    "SHD_ATL_064"})
		}
	}

	logger.Info("get redirect_url", "redirect_url", redirect_url)
	return redirect_url
}
