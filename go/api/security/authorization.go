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
	userInfo, err := rc.IsAuthenticated()
	if err != nil {
		logger.Error("authentication failed during permission check",
			"error", err,
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

	// TODO: Implement more granular permission checking based on rsc_id
	return nil
	/*
		// For now, non-admin/non-owner users are denied access to admin-only resources
		logger.Warn("permission denied",
			"user_id", userInfo.UserId,
			"rsc_type", rsc_type,
			"rsc_id", rsc_id,
			"rsc_opr", rsc_opr,
			"admin", userInfo.Admin,
			"is_owner", userInfo.IsOwner)

		return fmt.Errorf("permission denied for resource: %s", rsc_id)
	*/
}
