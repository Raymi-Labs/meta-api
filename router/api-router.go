package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	// Import oauth package to register providers via init()
	_ "github.com/QuantumNous/new-api/oauth"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func SetApiRouter(router *gin.Engine) {
	// Register separately from gzip: each synthetic SSE frame must flush immediately.
	router.GET("/api/route-probe", middleware.RouteTag("api"), middleware.RouteProbeRateLimit(false), controller.RouteProbe)
	router.GET("/api/route-probe/stream", middleware.RouteTag("api"), middleware.RouteProbeRateLimit(true), controller.RouteProbeStream)
	apiRouter := router.Group("/api")
	apiRouter.Use(middleware.RouteTag("api"))
	apiRouter.Use(gzip.Gzip(gzip.DefaultCompression))
	apiRouter.Use(middleware.BodyStorageCleanup()) // 清理请求体存储
	apiRouter.Use(middleware.GlobalAPIRateLimit())
	anonymousRequestBodyLimit := middleware.AnonymousRequestBodyLimit()
	{
		apiRouter.GET("/setup", controller.GetSetup)
		apiRouter.POST("/setup", anonymousRequestBodyLimit, controller.PostSetup)
		apiRouter.GET("/status", controller.GetStatus)
		apiRouter.GET("/uptime/status", controller.GetUptimeKumaStatus)
		apiRouter.GET("/monitor/channels", middleware.AdminAuth(), controller.GetChannelMonitor)
		apiRouter.GET("/health/groups", middleware.UserAuth(), controller.GetGroupHealth)
		apiRouter.GET("/models", middleware.UserAuth(), controller.DashboardListModels)
		apiRouter.GET("/status/test", middleware.AdminAuth(), controller.TestStatus)
		apiRouter.GET("/status/update", middleware.AdminAuth(), controller.CheckUpdate)
		apiRouter.GET("/notice", controller.GetNotice)
		apiRouter.GET("/user-agreement", controller.GetUserAgreement)
		apiRouter.GET("/privacy-policy", controller.GetPrivacyPolicy)
		apiRouter.GET("/about", controller.GetAbout)
		//apiRouter.GET("/midjourney", controller.GetMidjourney)
		apiRouter.GET("/home_page_content", controller.GetHomePageContent)
		// Meta Pulse integrations are isolated from the relay and dashboard auth.
		// The benefit receiver is service-signed; the forum SSO start endpoint
		// derives identity from the existing new-api session.
		pulseBenefitRoute := apiRouter.Group("/internal/pulse/benefits")
		{
			pulseBenefitRoute.POST("/grant", middleware.PulseServiceAuth(), middleware.PulseBenefitRateLimit(), controller.GrantPulseBenefit)
			pulseBenefitRoute.POST("/query", middleware.PulseBenefitQueryAuth(), middleware.PulseBenefitRateLimit(), controller.QueryPulseBenefit)
			pulseBenefitRoute.GET("/query/:source_ref", middleware.PulseBenefitQueryAuth(), middleware.PulseBenefitRateLimit(), controller.QueryPulseBenefit)
			pulseBenefitRoute.POST("/rollback", middleware.PulseRollbackAuth(), middleware.PulseBenefitRateLimit(), controller.RollbackPulseBenefit)
		}
		apiRouter.GET("/forum/sso/start", middleware.CriticalRateLimit(), controller.ForumSSOStart)
		pulseUserRoute := apiRouter.Group("/pulse")
		pulseUserRoute.Use(middleware.UserAuth())
		{
			pulseUserRoute.GET("/summary", controller.GetPulseSummary)
			pulseUserRoute.GET("/rewards", controller.GetPulseRewards)
		}
		pulseOpsRoute := apiRouter.Group("/pulse/ops")
		pulseOpsRoute.Use(middleware.AdminAuth())
		{
			pulseOpsRoute.GET("/overview", controller.GetPulseOperationsOverview)
			pulseOpsRoute.GET("/secret/generate", controller.GeneratePulseSecret)
		}
		apiRouter.POST("/usage/public_token", middleware.PublicTokenUsageRateLimit(), controller.GetPublicTokenUsage)
		apiRouter.POST("/usage/public_token/batch", middleware.PublicTokenUsageRateLimit(), controller.GetPublicTokenBatchUsage)
		apiRouter.POST("/usage/public_token/stats", middleware.PublicTokenUsageRateLimit(), controller.GetPublicTokenStats)
		apiRouter.POST("/usage/public_token/stats/batch", middleware.PublicTokenUsageRateLimit(), controller.GetPublicTokenBatchStats)
		apiRouter.POST("/usage/public_token/logs", middleware.PublicTokenUsageRateLimit(), controller.GetPublicTokenLogs)
		apiRouter.GET("/pricing", middleware.TryUserAuth(), controller.GetPricing)
		apiRouter.POST("/image-playground/session", middleware.UserAuth(), controller.CreateImagePlaygroundSession)
		perfMetricsRoute := apiRouter.Group("/perf-metrics")
		perfMetricsRoute.Use(middleware.TryUserAuth())
		{
			perfMetricsRoute.GET("/summary", controller.GetPerfMetricsSummary)
			perfMetricsRoute.GET("", controller.GetPerfMetrics)
		}
		apiRouter.GET("/verification", middleware.EmailVerificationRateLimit(), middleware.TurnstileCheck(), controller.SendEmailVerification)
		apiRouter.GET("/reset_password", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.SendPasswordResetEmail)
		apiRouter.POST("/user/reset", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.ResetPassword)
		// OAuth routes - specific routes must come before :provider wildcard
		apiRouter.GET("/oauth/state", middleware.CriticalRateLimit(), controller.GenerateOAuthCode)
		apiRouter.GET("/oauth/email/bind", middleware.CriticalRateLimit(), controller.EmailBind)
		apiRouter.POST("/oauth/email/bind", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.EmailBind)
		// Non-standard OAuth (WeChat, Telegram) - keep original routes
		apiRouter.GET("/oauth/wechat", middleware.CriticalRateLimit(), controller.WeChatAuth)
		apiRouter.GET("/oauth/wechat/bind", middleware.CriticalRateLimit(), controller.WeChatBind)
		apiRouter.POST("/oauth/wechat/bind", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.WeChatBind)
		apiRouter.GET("/oauth/telegram/login", middleware.CriticalRateLimit(), controller.TelegramLogin)
		apiRouter.GET("/oauth/telegram/bind", middleware.CriticalRateLimit(), controller.TelegramBind)
		// Standard OAuth providers (GitHub, Discord, OIDC, LinuxDO) - unified route
		apiRouter.GET("/oauth/:provider", middleware.CriticalRateLimit(), controller.HandleOAuth)
		apiRouter.GET("/ratio_config", middleware.CriticalRateLimit(), controller.GetRatioConfig)

		apiRouter.POST("/stripe/webhook", anonymousRequestBodyLimit, controller.StripeWebhook)
		apiRouter.POST("/creem/webhook", anonymousRequestBodyLimit, controller.CreemWebhook)
		apiRouter.POST("/waffo/webhook", anonymousRequestBodyLimit, controller.WaffoWebhook)
		// apiRouter.POST("/waffo-pancake/webhook", controller.WaffoPancakeWebhook)

		// Universal secure verification routes
		apiRouter.POST("/verify", middleware.UserAuth(), middleware.CriticalRateLimit(), controller.UniversalVerify)

		userRoute := apiRouter.Group("/user")
		{
			userRoute.POST("/register", middleware.RegisterRateLimit(), anonymousRequestBodyLimit, middleware.RegisterTurnstileCheck(), controller.Register)
			userRoute.POST("/login", middleware.TurnstileCheck(), middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.Login)
			userRoute.POST("/login/2fa", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.Verify2FALogin)
			userRoute.POST("/passkey/login/begin", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.PasskeyLoginBegin)
			userRoute.POST("/passkey/login/finish", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.PasskeyLoginFinish)
			//userRoute.POST("/tokenlog", middleware.CriticalRateLimit(), controller.TokenLog)
			userRoute.GET("/logout", controller.Logout)
			userRoute.POST("/epay/notify", anonymousRequestBodyLimit, controller.EpayNotify)
			userRoute.GET("/epay/notify", controller.EpayNotify)
			userRoute.POST("/epay/return", controller.EpayReturn)
			userRoute.GET("/epay/return", controller.EpayReturn)
			userRoute.GET("/groups", controller.GetUserGroups)

			selfRoute := userRoute.Group("/")
			selfRoute.Use(middleware.UserAuth())
			{
				selfRoute.GET("/self/groups", controller.GetUserGroups)
				selfRoute.GET("/self", controller.GetSelf)
				selfRoute.GET("/payment-records/self", controller.GetUserPaymentRecords)
				selfRoute.GET("/invoices/eligible-orders", controller.GetEligibleInvoiceOrders)
				selfRoute.POST("/invoices", controller.CreateInvoiceRequest)
				selfRoute.GET("/invoices/self", controller.GetUserInvoiceRequests)
				selfRoute.GET("/invoices/self/:id/file", controller.DownloadUserInvoiceFile)
				selfRoute.GET("/invoices/self/:id", controller.GetUserInvoiceRequestDetail)
				selfRoute.GET("/models", controller.GetUserModels)
				selfRoute.PUT("/self", controller.UpdateSelf)
				selfRoute.DELETE("/self", controller.DeleteSelf)
				selfRoute.GET("/token", controller.GenerateAccessToken)
				selfRoute.GET("/passkey", controller.PasskeyStatus)
				selfRoute.POST("/passkey/register/begin", controller.PasskeyRegisterBegin)
				selfRoute.POST("/passkey/register/finish", controller.PasskeyRegisterFinish)
				selfRoute.POST("/passkey/verify/begin", controller.PasskeyVerifyBegin)
				selfRoute.POST("/passkey/verify/finish", controller.PasskeyVerifyFinish)
				selfRoute.DELETE("/passkey", controller.PasskeyDelete)
				selfRoute.GET("/aff", controller.GetAffCode)
				selfRoute.GET("/invite-promotion/self", controller.GetSelfInvitePromotion)
				selfRoute.PUT("/aff", controller.SetAffCode)
				selfRoute.GET("/invite-recharge-commissions/self", controller.GetSelfInviteRechargeCommissions)
				selfRoute.GET("/topup/info", controller.GetTopUpInfo)
				selfRoute.GET("/topup/self", controller.GetUserTopUps)
				selfRoute.POST("/redeem", middleware.CriticalRateLimit(), controller.Redeem)
				selfRoute.POST("/redemptions", middleware.CriticalRateLimit(), middleware.WalletRedemptionRateLimit(), controller.CreateSelfRedemption)
				selfRoute.GET("/redemptions/self", controller.GetSelfRedemptions)
				selfRoute.GET("/redemptions/self/summary", controller.GetSelfRedemptionUsageSummary)
				selfRoute.POST("/topup", middleware.CriticalRateLimit(), controller.TopUp)
				selfRoute.POST("/pay", middleware.CriticalRateLimit(), controller.RequestEpay)
				selfRoute.POST("/amount", controller.RequestAmount)
				selfRoute.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.RequestStripePay)
				selfRoute.POST("/stripe/amount", controller.RequestStripeAmount)
				selfRoute.POST("/creem/pay", middleware.CriticalRateLimit(), controller.RequestCreemPay)
				selfRoute.POST("/waffo/pay", middleware.CriticalRateLimit(), controller.RequestWaffoPay)
				selfRoute.POST("/waffo/amount", controller.RequestWaffoAmount)
				// selfRoute.POST("/waffo-pancake/pay", middleware.CriticalRateLimit(), controller.RequestWaffoPancakePay)
				// selfRoute.POST("/waffo-pancake/amount", controller.RequestWaffoPancakeAmount)
				selfRoute.POST("/aff_transfer", controller.TransferAffQuota)
				selfRoute.POST("/aff-withdrawals", middleware.CriticalRateLimit(), controller.CreateAffWithdrawal)
				selfRoute.GET("/aff-withdrawals/self", controller.GetUserAffWithdrawals)
				selfRoute.PUT("/setting", controller.UpdateUserSetting)
				selfRoute.GET("/quota-notices", controller.GetSelfQuotaNotices)

				// 2FA routes
				selfRoute.GET("/2fa/status", controller.Get2FAStatus)
				selfRoute.POST("/2fa/setup", controller.Setup2FA)
				selfRoute.POST("/2fa/enable", controller.Enable2FA)
				selfRoute.POST("/2fa/disable", controller.Disable2FA)
				selfRoute.POST("/2fa/backup_codes", controller.RegenerateBackupCodes)

				// Check-in routes
				selfRoute.GET("/checkin", controller.GetCheckinStatus)
				selfRoute.POST("/checkin", middleware.TurnstileCheck(), controller.DoCheckin)

				// Custom OAuth bindings
				selfRoute.GET("/oauth/bindings", controller.GetUserOAuthBindings)
				selfRoute.DELETE("/oauth/bindings/:provider_id", controller.UnbindCustomOAuth)
			}

			adminRoute := userRoute.Group("/")
			adminRoute.Use(middleware.AdminAuth())
			{
				adminRoute.GET("/", controller.GetAllUsers)
				adminRoute.GET("/payment-records", controller.GetAllPaymentRecords)
				adminRoute.GET("/payment-records/stats", controller.GetPaymentRecordStats)
				adminRoute.GET("/payment-records/rankings", controller.GetPaymentRecordRankings)
				adminRoute.GET("/invoices", controller.GetAllInvoiceRequests)
				adminRoute.GET("/invoices/:id/file", controller.DownloadInvoiceFile)
				adminRoute.GET("/invoices/:id", controller.GetInvoiceRequestDetail)
				adminRoute.POST("/invoices/:id/approve", controller.ApproveInvoiceRequest)
				adminRoute.POST("/invoices/:id/reject", controller.RejectInvoiceRequest)
				adminRoute.POST("/invoices/:id/resend-email", controller.ResendInvoiceEmail)
				adminRoute.GET("/aff-withdrawals", controller.GetAllAffWithdrawals)
				adminRoute.POST("/aff-withdrawals/:id/approve", controller.ApproveAffWithdrawal)
				adminRoute.POST("/aff-withdrawals/:id/reject", controller.RejectAffWithdrawal)
				adminRoute.GET("/payment-risk-cases", controller.GetAllPaymentRiskCases)
				adminRoute.GET("/payment-risk-cases/:id", controller.GetPaymentRiskCaseDetail)
				adminRoute.POST("/payment-risk-cases", controller.CreatePaymentRiskCase)
				adminRoute.POST("/payment-risk-cases/:id/resolve", controller.ResolvePaymentRiskCase)
				adminRoute.GET("/topup", controller.GetAllTopUps)
				adminRoute.POST("/topup/complete", controller.AdminCompleteTopUp)
				adminRoute.GET("/search", controller.SearchUsers)
				adminRoute.POST("/rebuild-aff-count", controller.RebuildAffCount)
				adminRoute.GET("/ip-blacklist", controller.GetIPBlacklists)
				adminRoute.POST("/ip-blacklist", controller.CreateIPBlacklist)
				adminRoute.POST("/ip-blacklist/batch", controller.BatchCreateIPBlacklist)
				adminRoute.GET("/ip-blacklist/:id/users", controller.GetIPBlacklistUsers)
				adminRoute.POST("/ip-blacklist/:id/users/manage", controller.ManageIPBlacklistUsers)
				adminRoute.DELETE("/ip-blacklist/:id", controller.DeleteIPBlacklist)
				adminRoute.GET("/:id/invite-relations", controller.GetUserInviteRelations)
				adminRoute.GET("/:id/quota-notices", controller.GetUserQuotaNotices)
				adminRoute.GET("/:id/invite-promotion", controller.GetAdminInvitePromotion)
				adminRoute.GET("/:id/oauth/bindings", controller.GetUserOAuthBindingsByAdmin)
				adminRoute.DELETE("/:id/oauth/bindings/:provider_id", controller.UnbindCustomOAuthByAdmin)
				adminRoute.DELETE("/:id/bindings/:binding_type", controller.AdminClearUserBinding)
				adminRoute.GET("/:id", controller.GetUser)
				adminRoute.POST("/", controller.CreateUser)
				adminRoute.POST("/manage", controller.ManageUser)
				adminRoute.POST("/manage/batch", controller.ManageUserBatch)
				adminRoute.PUT("/", controller.UpdateUser)
				adminRoute.DELETE("/:id", controller.DeleteUser)
				adminRoute.DELETE("/:id/reset_passkey", controller.AdminResetPasskey)

				// Admin 2FA routes
				adminRoute.GET("/2fa/stats", controller.Admin2FAStats)
				adminRoute.DELETE("/:id/2fa", controller.AdminDisable2FA)
			}
		}

		// Subscription billing (plans, purchase, admin management)
		subscriptionRoute := apiRouter.Group("/subscription")
		subscriptionRoute.Use(middleware.UserAuth())
		{
			subscriptionRoute.GET("/plans", controller.GetSubscriptionPlans)
			subscriptionRoute.GET("/self", controller.GetSubscriptionSelf)
			subscriptionRoute.GET("/self/issuances", controller.ListSubscriptionIssuances)
			subscriptionRoute.GET("/self/issuances/:id", controller.GetSubscriptionIssuance)
			subscriptionRoute.POST("/self/issuances/:id/confirm", middleware.CriticalRateLimit(), controller.ConfirmSubscriptionIssuance)
			subscriptionRoute.PUT("/self/preference", controller.UpdateSubscriptionPreference)
			subscriptionRoute.POST("/epay/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestEpay)
			subscriptionRoute.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestStripePay)
			subscriptionRoute.POST("/creem/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestCreemPay)
		}
		subscriptionAdminRoute := apiRouter.Group("/subscription/admin")
		subscriptionAdminRoute.Use(middleware.AdminAuth())
		{
			subscriptionAdminRoute.GET("/plans", controller.AdminListSubscriptionPlans)
			subscriptionAdminRoute.POST("/plans", controller.AdminCreateSubscriptionPlan)
			subscriptionAdminRoute.PUT("/plans/:id", controller.AdminUpdateSubscriptionPlan)
			subscriptionAdminRoute.PATCH("/plans/:id", controller.AdminUpdateSubscriptionPlanStatus)
			subscriptionAdminRoute.POST("/bind", controller.AdminBindSubscription)

			// User subscription management (admin)
			subscriptionAdminRoute.GET("/users/:id/subscriptions", controller.AdminListUserSubscriptions)
			subscriptionAdminRoute.POST("/users/:id/subscriptions", controller.AdminCreateUserSubscription)
			subscriptionAdminRoute.POST("/users/:id/subscriptions/manage", controller.AdminManageUserSubscription)
			subscriptionAdminRoute.POST("/users/:id/subscriptions/manage/batch", controller.AdminManageUserSubscriptionBatch)
			subscriptionAdminRoute.POST("/user_subscriptions/:id/invalidate", controller.AdminInvalidateUserSubscription)
			subscriptionAdminRoute.DELETE("/user_subscriptions/:id", controller.AdminDeleteUserSubscription)
		}

		// Subscription payment callbacks (no auth)
		apiRouter.POST("/subscription/epay/notify", anonymousRequestBodyLimit, controller.SubscriptionEpayNotify)
		apiRouter.GET("/subscription/epay/notify", controller.SubscriptionEpayNotify)
		apiRouter.GET("/subscription/epay/return", controller.SubscriptionEpayReturn)
		apiRouter.POST("/subscription/epay/return", anonymousRequestBodyLimit, controller.SubscriptionEpayReturn)
		optionRoute := apiRouter.Group("/option")
		optionRoute.Use(middleware.RootAuth())
		{
			optionRoute.GET("/", controller.GetOptions)
			optionRoute.GET("/pulse", controller.GetPulseOptions)
			optionRoute.PUT("/pulse", controller.UpdatePulseOptions)
			optionRoute.PUT("/", controller.UpdateOption)
			optionRoute.GET("/channel_affinity_cache", controller.GetChannelAffinityCacheStats)
			optionRoute.DELETE("/channel_affinity_cache", controller.ClearChannelAffinityCache)
			optionRoute.GET("/slow_ttft_guard", controller.GetSlowTTFTGuardStats)
			optionRoute.POST("/slow_ttft_guard/refresh", controller.RefreshSlowTTFTBaselines)
			optionRoute.DELETE("/slow_ttft_guard", controller.ClearSlowTTFTGuardState)
			optionRoute.GET("/time_ratio_preview", controller.GetTimeRatioPreview)
			optionRoute.POST("/rest_model_ratio", controller.ResetModelRatio)
			optionRoute.GET("/model_permissions", controller.ListModelPermissions)
			optionRoute.POST("/model_permissions", controller.CreateModelPermission)
			optionRoute.GET("/model_permissions/candidates", controller.GetModelPermissionCandidates)
			optionRoute.PUT("/model_permissions/:id", controller.UpdateModelPermission)
			optionRoute.DELETE("/model_permissions/:id", controller.DeleteModelPermission)
			optionRoute.POST("/migrate_console_setting", controller.MigrateConsoleSetting) // 用于迁移检测的旧键，下个版本会删除
		}

		// Custom OAuth provider management (root only)
		customOAuthRoute := apiRouter.Group("/custom-oauth-provider")
		customOAuthRoute.Use(middleware.RootAuth())
		{
			customOAuthRoute.POST("/discovery", controller.FetchCustomOAuthDiscovery)
			customOAuthRoute.GET("/", controller.GetCustomOAuthProviders)
			customOAuthRoute.GET("/:id", controller.GetCustomOAuthProvider)
			customOAuthRoute.POST("/", controller.CreateCustomOAuthProvider)
			customOAuthRoute.PUT("/:id", controller.UpdateCustomOAuthProvider)
			customOAuthRoute.DELETE("/:id", controller.DeleteCustomOAuthProvider)
		}
		performanceRoute := apiRouter.Group("/performance")
		performanceRoute.Use(middleware.RootAuth())
		{
			performanceRoute.GET("/stats", controller.GetPerformanceStats)
			performanceRoute.DELETE("/disk_cache", controller.ClearDiskCache)
			performanceRoute.POST("/reset_stats", controller.ResetPerformanceStats)
			performanceRoute.POST("/gc", controller.ForceGC)
		}
		channelConcurrencyRoute := apiRouter.Group("/channel_concurrency")
		channelConcurrencyRoute.Use(middleware.AdminAuth())
		{
			channelConcurrencyRoute.GET("/stats", controller.GetChannelConcurrencyStats)
		}
		ratioSyncRoute := apiRouter.Group("/ratio_sync")
		ratioSyncRoute.Use(middleware.RootAuth())
		{
			ratioSyncRoute.GET("/channels", controller.GetSyncableChannels)
			ratioSyncRoute.POST("/fetch", controller.FetchUpstreamRatios)
		}
		channelRoute := apiRouter.Group("/channel")
		channelRoute.Use(middleware.AdminAuth())
		{
			channelRoute.GET("/", controller.GetAllChannels)
			channelRoute.GET("/search", controller.SearchChannels)
			channelRoute.GET("/usage", controller.GetChannelUsage)
			channelRoute.GET("/models", controller.ChannelListModels)
			channelRoute.GET("/models_enabled", controller.EnabledListModels)
			channelRoute.GET("/:id", controller.GetChannel)
			channelRoute.GET("/:id/quota_usage", controller.GetChannelQuotaUsage)
			channelRoute.POST("/:id/key", middleware.RootAuth(), middleware.CriticalRateLimit(), middleware.DisableCache(), middleware.SecureVerificationRequired(), controller.GetChannelKey)
			channelRoute.POST("/:id/manual_enable", controller.ManualEnableChannel)
			channelRoute.GET("/test", controller.TestAllChannels)
			channelRoute.GET("/test/:id", controller.TestChannel)
			channelRoute.POST("/:id/video/preflight", controller.VideoChannelPreflight)
			channelRoute.GET("/update_balance", controller.UpdateAllChannelsBalance)
			channelRoute.GET("/update_balance/:id", controller.UpdateChannelBalance)
			channelRoute.POST("/", controller.AddChannel)
			channelRoute.PUT("/", controller.UpdateChannel)
			channelRoute.DELETE("/disabled", controller.DeleteDisabledChannel)
			channelRoute.GET("/tag/quota_policy", controller.GetChannelTagQuotaPolicy)
			channelRoute.PUT("/tag/quota_policy", controller.PutChannelTagQuotaPolicy)
			channelRoute.DELETE("/tag/quota_policy", controller.DeleteChannelTagQuotaPolicy)
			channelRoute.GET("/tag/quota_usage", controller.GetChannelTagQuotaUsage)
			channelRoute.POST("/tag/disabled", controller.DisableTagChannels)
			channelRoute.POST("/tag/enabled", controller.EnableTagChannels)
			channelRoute.PUT("/tag", controller.EditTagChannels)
			channelRoute.DELETE("/:id", controller.DeleteChannel)
			channelRoute.POST("/batch", controller.DeleteChannelBatch)
			channelRoute.POST("/fix", controller.FixChannelsAbilities)
			channelRoute.POST("/upstream_updates/apply", controller.ApplyChannelUpstreamModelUpdates)
			channelRoute.POST("/upstream_updates/apply_all", controller.ApplyAllChannelUpstreamModelUpdates)
			channelRoute.POST("/upstream_updates/detect", controller.DetectChannelUpstreamModelUpdates)
			channelRoute.POST("/upstream_updates/detect_all", controller.DetectAllChannelUpstreamModelUpdates)
			channelRoute.GET("/fetch_models/:id", controller.FetchUpstreamModels)
			channelRoute.POST("/fetch_models", controller.FetchModels)
			channelRoute.POST("/codex/oauth/start", controller.StartCodexOAuth)
			channelRoute.POST("/codex/oauth/complete", controller.CompleteCodexOAuth)
			channelRoute.POST("/:id/codex/oauth/start", controller.StartCodexOAuthForChannel)
			channelRoute.POST("/:id/codex/oauth/complete", controller.CompleteCodexOAuthForChannel)
			channelRoute.POST("/:id/codex/refresh", controller.RefreshCodexChannelCredential)
			channelRoute.GET("/:id/codex/usage", controller.GetCodexChannelUsage)
			channelRoute.POST("/ollama/pull", controller.OllamaPullModel)
			channelRoute.POST("/ollama/pull/stream", controller.OllamaPullModelStream)
			channelRoute.DELETE("/ollama/delete", controller.OllamaDeleteModel)
			channelRoute.GET("/ollama/version/:id", controller.OllamaVersion)
			channelRoute.POST("/batch/tag", controller.BatchSetChannelTag)
			channelRoute.GET("/tag/models", controller.GetTagModels)
			channelRoute.GET("/tag/channels", controller.GetTagChannels)
			channelRoute.POST("/copy/:id", controller.CopyChannel)
			channelRoute.POST("/multi_key/manage", controller.ManageMultiKeys)
			channelRoute.GET("/known_clients", controller.GetKnownClients)
		}
		upstreamAccountRoute := apiRouter.Group("/upstream_accounts")
		upstreamAccountRoute.Use(middleware.AdminAuth())
		{
			upstreamAccountRoute.GET("", controller.GetUpstreamAccounts)
			upstreamAccountRoute.POST("", controller.SaveUpstreamAccount)
			upstreamAccountRoute.PUT("/:id", controller.SaveUpstreamAccount)
			upstreamAccountRoute.DELETE("/:id", controller.DeleteUpstreamAccount)
			upstreamAccountRoute.POST("/sync_all", controller.SyncAllUpstreamAccounts)
			upstreamAccountRoute.POST("/:id/sync", controller.SyncUpstreamAccount)
			upstreamAccountRoute.GET("/:id/trend", controller.GetUpstreamAccountTrend)
		}

		// Keep the account-only legacy path for one compatibility cycle.
		legacyUpstreamAccountRoute := apiRouter.Group("/profit_board")
		legacyUpstreamAccountRoute.Use(middleware.AdminAuth())
		{
			legacyUpstreamAccountRoute.GET("/upstream_accounts", controller.GetUpstreamAccounts)
			legacyUpstreamAccountRoute.POST("/upstream_accounts", controller.SaveUpstreamAccount)
			legacyUpstreamAccountRoute.PUT("/upstream_accounts/:id", controller.SaveUpstreamAccount)
			legacyUpstreamAccountRoute.DELETE("/upstream_accounts/:id", controller.DeleteUpstreamAccount)
			legacyUpstreamAccountRoute.POST("/upstream_accounts/sync_all", controller.SyncAllUpstreamAccounts)
			legacyUpstreamAccountRoute.POST("/upstream_accounts/:id/sync", controller.SyncUpstreamAccount)
			legacyUpstreamAccountRoute.GET("/upstream_accounts/:id/trend", controller.GetUpstreamAccountTrend)
		}
		crsRoute := apiRouter.Group("/crs")
		crsRoute.Use(middleware.AdminAuth())
		{
			crsRoute.GET("/overview", controller.GetCRSOverview)
			crsRoute.GET("/accounts", controller.GetCRSAccounts)
			crsRoute.GET("/sites", controller.GetCRSSites)
			crsRoute.GET("/sites/:id/accounts", controller.GetCRSSiteAccounts)
			crsRoute.POST("/sites", controller.CreateCRSSite)
			crsRoute.PUT("/sites/:id", controller.UpdateCRSSite)
			crsRoute.DELETE("/sites/:id", controller.DeleteCRSSite)
			crsRoute.POST("/sites/:id/refresh", controller.RefreshCRSSiteByID)
			crsRoute.POST("/refresh_all", controller.RefreshAllCRSSites)
		}
		cpaRoute := apiRouter.Group("/cpa")
		cpaRoute.Use(middleware.AdminAuth())
		{
			cpaRoute.GET("/overview", controller.GetCPAOverview)
			cpaRoute.POST("/test_connection", controller.TestCPAConnection)
			cpaRoute.POST("/sites", controller.CreateCPASite)
			cpaRoute.PUT("/sites/:id", controller.UpdateCPASite)
			cpaRoute.DELETE("/sites/:id", controller.DeleteCPASite)
			cpaRoute.POST("/sites/:id/refresh", controller.RefreshCPASite)
		}
		tokenRoute := apiRouter.Group("/token")
		tokenRoute.Use(middleware.UserAuth())
		{
			tokenRoute.GET("/", controller.GetAllTokens)
			tokenRoute.GET("/search", middleware.SearchRateLimit(), controller.SearchTokens)
			tokenRoute.GET("/models", controller.GetTokenModels)
			tokenRoute.GET("/channels", middleware.AdminAuth(), controller.GetTokenChannels)
			tokenRoute.POST("/test/:id", controller.TestToken)
			tokenRoute.GET("/:id/key", controller.GetTokenKey)
			tokenRoute.GET("/:id", controller.GetToken)
			tokenRoute.POST("/", controller.AddToken)
			tokenRoute.PUT("/", controller.UpdateToken)
			tokenRoute.DELETE("/:id", controller.DeleteToken)
			tokenRoute.POST("/batch", controller.DeleteTokenBatch)
			tokenRoute.POST("/manage/batch", controller.ManageTokenBatch)
			tokenRoute.POST("/runtime_status", controller.GetTokensRuntimeStatus)
		}

		usageRoute := apiRouter.Group("/usage")
		usageRoute.Use(middleware.CORS(), middleware.CriticalRateLimit())
		{
			tokenUsageRoute := usageRoute.Group("/token")
			tokenUsageRoute.Use(middleware.TokenAuthQuery())
			{
				tokenUsageRoute.GET("/", controller.GetTokenUsage)
			}
		}

		redemptionRoute := apiRouter.Group("/redemption")
		redemptionRoute.Use(middleware.AdminAuth())
		{
			redemptionRoute.GET("/", controller.GetAllRedemptions)
			redemptionRoute.GET("/search", controller.SearchRedemptions)
			redemptionRoute.GET("/review-cases", controller.GetRedemptionReviewCases)
			redemptionRoute.GET("/review-cases/summary", controller.GetRedemptionReviewSummary)
			redemptionRoute.POST("/review-cases/:id/resolve", controller.ResolveRedemptionReviewCase)
			redemptionRoute.GET("/:id", controller.GetRedemption)
			redemptionRoute.POST("/", controller.AddRedemption)
			redemptionRoute.POST("/manage/batch", controller.ManageRedemptionBatch)
			redemptionRoute.PUT("/", controller.UpdateRedemption)
			redemptionRoute.DELETE("/invalid", controller.DeleteInvalidRedemption)
			redemptionRoute.DELETE("/:id", controller.DeleteRedemption)
		}
		logRoute := apiRouter.Group("/log")
		logRoute.GET("/", middleware.AdminAuth(), controller.GetAllLogs)
		logRoute.DELETE("/", middleware.AdminAuth(), controller.DeleteHistoryLogs)
		logRoute.GET("/stat", middleware.AdminAuth(), controller.GetLogsStat)
		logRoute.GET("/top-users", middleware.AdminAuth(), controller.GetTopUsers)
		logRoute.GET("/channel-options", middleware.AdminAuth(), controller.GetLogChannelOptions)
		logRoute.GET("/group-summary", middleware.AdminAuth(), controller.GetLogGroupSummary)
		logRoute.GET("/self/stat", middleware.UserAuth(), controller.GetLogsSelfStat)
		logRoute.GET("/channel_affinity_usage_cache", middleware.AdminAuth(), controller.GetChannelAffinityUsageCacheStats)
		logRoute.GET("/search", middleware.AdminAuth(), controller.SearchAllLogs)
		logRoute.GET("/self", middleware.UserAuth(), controller.GetUserLogs)
		logRoute.GET("/export", middleware.AdminAuth(), controller.ExportAllLogs)
		logRoute.GET("/self/export", middleware.UserAuth(), controller.ExportUserLogs)
		logRoute.GET("/self/search", middleware.UserAuth(), middleware.SearchRateLimit(), controller.SearchUserLogs)
		contentSafetyRoute := apiRouter.Group("/content-safety")
		{
			contentSafetyRoute.GET("/self", middleware.UserAuth(), controller.GetSelfContentSafetyState)
			contentSafetyRoute.POST("/self/acknowledge", middleware.UserAuth(), controller.AcknowledgeSelfContentSafetyWarnings)
			contentSafetyRoute.GET("/users/:id/whitelist", middleware.AdminAuth(), controller.GetContentSafetyWhitelist)
			contentSafetyRoute.PUT("/users/:id/whitelist", middleware.AdminAuth(), controller.UpdateContentSafetyWhitelist)
			contentSafetyRoute.GET("/violations", middleware.AdminAuth(), controller.GetContentSafetyViolations)
			contentSafetyRoute.GET("/violations/:id/evidence", middleware.AdminAuth(), controller.GetContentSafetyViolationEvidence)
			contentSafetyRoute.GET("/review-cases", middleware.AdminAuth(), controller.GetContentSafetyReviewCases)
			contentSafetyRoute.POST("/review-cases/:id/resolve", middleware.AdminAuth(), controller.ResolveContentSafetyReviewCase)
		}

		dataRoute := apiRouter.Group("/data")
		dataRoute.GET("/", middleware.AdminAuth(), controller.GetAllQuotaDates)
		dataRoute.GET("/users", middleware.AdminAuth(), controller.GetQuotaDatesByUser)
		dataRoute.GET("/model-channel-tags", middleware.AdminAuth(), controller.GetModelChannelTagStats)
		dataRoute.GET("/user-balance-trend", middleware.AdminAuth(), controller.GetUserBalanceTrend)
		dataRoute.GET("/user-balance-trend/users", middleware.AdminAuth(), controller.GetUserBalanceTrendUsers)
		dataRoute.PUT("/user-balance-trend/users/:id", middleware.AdminAuth(), controller.UpdateUserBalanceTrendUser)
		dataRoute.GET("/self", middleware.UserAuth(), controller.GetUserQuotaDates)

		logRoute.Use(middleware.CORS(), middleware.CriticalRateLimit())
		{
			logRoute.GET("/token", middleware.TokenAuthQuery(), controller.GetLogByKey)
		}
		groupRoute := apiRouter.Group("/group")
		groupRoute.Use(middleware.AdminAuth())
		{
			groupRoute.GET("/", controller.GetGroups)
		}

		prefillGroupRoute := apiRouter.Group("/prefill_group")
		prefillGroupRoute.Use(middleware.AdminAuth())
		{
			prefillGroupRoute.GET("/", controller.GetPrefillGroups)
			prefillGroupRoute.POST("/", controller.CreatePrefillGroup)
			prefillGroupRoute.PUT("/", controller.UpdatePrefillGroup)
			prefillGroupRoute.DELETE("/:id", controller.DeletePrefillGroup)
		}

		mjRoute := apiRouter.Group("/mj")
		mjRoute.GET("/self", middleware.UserAuth(), controller.GetUserMidjourney)
		mjRoute.GET("/", middleware.AdminAuth(), controller.GetAllMidjourney)

		taskRoute := apiRouter.Group("/task")
		{
			taskRoute.GET("/self", middleware.UserAuth(), controller.GetUserTask)
			taskRoute.GET("/", middleware.AdminAuth(), controller.GetAllTask)
		}

		vendorRoute := apiRouter.Group("/vendors")
		vendorRoute.Use(middleware.AdminAuth())
		{
			vendorRoute.GET("/", controller.GetAllVendors)
			vendorRoute.GET("/search", controller.SearchVendors)
			vendorRoute.GET("/:id", controller.GetVendorMeta)
			vendorRoute.POST("/", controller.CreateVendorMeta)
			vendorRoute.PUT("/", controller.UpdateVendorMeta)
			vendorRoute.DELETE("/:id", controller.DeleteVendorMeta)
		}

		modelsRoute := apiRouter.Group("/models")
		modelsRoute.Use(middleware.AdminAuth())
		{
			modelsRoute.GET("/sync_upstream/preview", controller.SyncUpstreamPreview)
			modelsRoute.POST("/sync_upstream", controller.SyncUpstreamModels)
			modelsRoute.GET("/missing", controller.GetMissingModels)
			modelsRoute.GET("/", controller.GetAllModelsMeta)
			modelsRoute.GET("/search", controller.SearchModelsMeta)
			modelsRoute.GET("/:id", controller.GetModelMeta)
			modelsRoute.POST("/", controller.CreateModelMeta)
			modelsRoute.PUT("/", controller.UpdateModelMeta)
			modelsRoute.DELETE("/:id", controller.DeleteModelMeta)
		}

		// Deployments (model deployment management)
		deploymentsRoute := apiRouter.Group("/deployments")
		deploymentsRoute.Use(middleware.AdminAuth())
		{
			deploymentsRoute.GET("/settings", controller.GetModelDeploymentSettings)
			deploymentsRoute.POST("/settings/test-connection", controller.TestIoNetConnection)
			deploymentsRoute.GET("/", controller.GetAllDeployments)
			deploymentsRoute.GET("/search", controller.SearchDeployments)
			deploymentsRoute.POST("/test-connection", controller.TestIoNetConnection)
			deploymentsRoute.GET("/hardware-types", controller.GetHardwareTypes)
			deploymentsRoute.GET("/locations", controller.GetLocations)
			deploymentsRoute.GET("/available-replicas", controller.GetAvailableReplicas)
			deploymentsRoute.POST("/price-estimation", controller.GetPriceEstimation)
			deploymentsRoute.GET("/check-name", controller.CheckClusterNameAvailability)
			deploymentsRoute.POST("/", controller.CreateDeployment)

			deploymentsRoute.GET("/:id", controller.GetDeployment)
			deploymentsRoute.GET("/:id/logs", controller.GetDeploymentLogs)
			deploymentsRoute.GET("/:id/containers", controller.ListDeploymentContainers)
			deploymentsRoute.GET("/:id/containers/:container_id", controller.GetContainerDetails)
			deploymentsRoute.PUT("/:id", controller.UpdateDeployment)
			deploymentsRoute.PUT("/:id/name", controller.UpdateDeploymentName)
			deploymentsRoute.POST("/:id/extend", controller.ExtendDeployment)
			deploymentsRoute.DELETE("/:id", controller.DeleteDeployment)
		}
	}
}
