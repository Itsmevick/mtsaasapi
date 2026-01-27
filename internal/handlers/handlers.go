package handlers

import (
	"github.com/multitenant-saas/api/internal/config"
	"github.com/multitenant-saas/api/internal/services"
)

type Handlers struct {
	AuthService      *services.AuthService
	WorkspaceService *services.WorkspaceService
	MemberService    *services.MemberService
	ProjectService   *services.ProjectService
	TaskService      *services.TaskService
	ActivityService  *services.ActivityService
	BillingService   *services.BillingService
	DashboardService *services.DashboardService
	Config           *config.Config
}

func New(
	authService *services.AuthService,
	workspaceService *services.WorkspaceService,
	memberService *services.MemberService,
	projectService *services.ProjectService,
	taskService *services.TaskService,
	activityService *services.ActivityService,
	billingService *services.BillingService,
	dashboardService *services.DashboardService,
	cfg *config.Config,
) *Handlers {
	return &Handlers{
		AuthService:      authService,
		WorkspaceService: workspaceService,
		MemberService:    memberService,
		ProjectService:    projectService,
		TaskService:       taskService,
		ActivityService:   activityService,
		BillingService:    billingService,
		DashboardService: dashboardService,
		Config:           cfg,
	}
}
