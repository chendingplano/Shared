package security

import (
	"fmt"
	"sync"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

var (
	accCtrlMgrInstance *AccCtrlMgr
	accCtrlMgrOnce     sync.Once
)

type AccCtrlMgr struct {
	rsc_map map[string]bool
	logger  *loggerutil.JimoLogger
}

// GetAccCtrlMgr returns the singleton instance of AccCtrlMgr.
// Must call InitAccCtrlMgr first to initialize the singleton.
func GetAccCtrlMgr() *AccCtrlMgr {
	return accCtrlMgrInstance
}

// InitAccCtrlMgr initializes the singleton instance of AccCtrlMgr.
// This should be called once during application startup.
func InitAccCtrlMgr() *AccCtrlMgr {
	accCtrlMgrOnce.Do(func() {
		logger := loggerutil.CreateLogger2(
			loggerutil.ContextTypeBackground,
			loggerutil.LogHandlerTypeDefault,
			10000)
		accCtrlMgrInstance = &AccCtrlMgr{
			rsc_map: make(map[string]bool),
			logger:  logger,
		}
	})
	return accCtrlMgrInstance
}

// NewAccCtrlMgr creates a new AccCtrlMgr instance
// Deprecated: Use InitAccCtrlMgr and GetAccCtrlMgr instead for singleton access
func NewAccCtrlMgr(logger *loggerutil.JimoLogger) *AccCtrlMgr {
	return &AccCtrlMgr{
		rsc_map: make(map[string]bool),
		logger:  logger,
	}
}

// Init reads configurations from databases and initializes the manager
func (m *AccCtrlMgr) Init() error {
	// TBD: Read configurations from databases
	return nil
}

// RequirePermission checks if the current user has permission to
// to perform 'opr' on the specified resource.
// Resource type is specified by rsc_type
// resource_id is normally a db name and table name.
// Returns nil if permission is granted, or an error if denied.
func (m *AccCtrlMgr) RequirePermission(
	rc ApiTypes.RequestContext,
	rsc_type ApiTypes.RscType,
	rsc_id string,
	rsc_opr ApiTypes.RscOpr) error {
	logger := m.logger

	// Authenticate the user first
	userInfo := rc.IsAuthenticated()
	if userInfo == nil {
		logger.Error("authentication failed during permission check",
			"rsc_type", rsc_type,
			"rsc_id", rsc_id,
			"rsc_opr", rsc_opr)
		return fmt.Errorf("authentication required")
	}

	// Admin and Owner have full access
	if userInfo.Admin || userInfo.IsOwner {
		logger.Info("permission granted",
			"user_id", userInfo.UserId,
			"rsc_type", rsc_type,
			"rsc_id", rsc_id,
			"rsc_opr", rsc_opr,
			"reason", "admin_or_owner")
		return nil
	}

	/* Chen Ding, 2026/01/27
	TBD
	// SECURITY FIX: Deny access by default for non-admin/non-owner users
	// This follows the principle of least privilege - grant only what is explicitly allowed
	// TODO: Implement granular permission checking when role-based access is needed
	logger.Warn("permission denied - non-admin/non-owner access attempt",
		"user_id", userInfo.UserId,
		"rsc_type", rsc_type,
		"rsc_id", rsc_id,
		"rsc_opr", rsc_opr,
		"admin", userInfo.Admin,
		"is_owner", userInfo.IsOwner)

	return fmt.Errorf("permission denied: insufficient privileges for resource %s (SHD_ATR_102)", rsc_id)
	*/
	return nil
}
