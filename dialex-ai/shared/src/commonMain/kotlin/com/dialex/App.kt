@file:Suppress("DEPRECATION")

package com.dialex

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInHorizontally
import androidx.compose.animation.slideOutHorizontally
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.dialex.data.datasource.EngineDataSource
import com.dialex.data.datasource.LocalDatabaseSource
import com.dialex.data.datasource.ProfileLocalDataSource
import com.dialex.data.datasource.SecureKeyDataSource
import com.dialex.data.db.defaultPlatformDatabase
import com.dialex.data.repository.ApiKeyRepositoryImpl
import com.dialex.data.datasource.TemplateLocalDataSource
import com.dialex.data.repository.DiscussionRepositoryImpl
import com.dialex.data.repository.LocalStoreRepositoryImpl
import com.dialex.data.repository.PersonaRepositoryImpl
import com.dialex.data.repository.ProfileRepositoryImpl
import com.dialex.data.repository.ProjectRepositoryImpl
import com.dialex.data.repository.SettingsRepositoryImpl
import com.dialex.data.repository.TemplateRepositoryImpl
import com.dialex.data.repository.GraphRepositoryImpl
import com.dialex.data.repository.BenchmarkRepositoryImpl
import com.dialex.domain.model.ConnectionProfile
import com.dialex.domain.repository.ApiKeyRepository
import com.dialex.domain.repository.DiscussionRepository
import com.dialex.domain.repository.LocalStoreRepository
import com.dialex.domain.repository.PersonaRepository
import com.dialex.domain.repository.ProfileRepository
import com.dialex.domain.repository.ProjectRepository
import com.dialex.domain.repository.SettingsRepository
import com.dialex.domain.repository.TemplateRepository
import com.dialex.domain.repository.GraphRepository
import com.dialex.domain.repository.BenchmarkRepository
import com.dialex.presentation.arena.BenchmarkRoute
import com.dialex.presentation.graph.GraphRoute
import com.dialex.presentation.legal.licensing.LicensingRoute
import com.dialex.presentation.nav.BenchmarkArena
import com.dialex.presentation.nav.Graph
import com.dialex.presentation.nav.LicensingDedication
import com.dialex.presentation.profile.ProfileLockDialog
import com.dialex.engine.EngineClient
import com.dialex.ui.LocalWindowMaximizeToggle
import com.dialex.export.exportFileName
import com.dialex.export.exportMemoFileName
import com.dialex.export.toExecutiveMemorandumHtml
import com.dialex.export.toMarkdown
import com.dialex.model.ApiKeys
import com.dialex.model.CliCommands
import com.dialex.model.Discussion
import com.dialex.model.DiscussionStatus
import com.dialex.model.Project
import com.dialex.presentation.chat.ChatRoute
import com.dialex.presentation.connect.ConnectRoute
import com.dialex.presentation.nav.AppBackHandler
import com.dialex.presentation.nav.AppBackStack
import com.dialex.presentation.nav.AppRoute
import com.dialex.presentation.nav.Chat
import com.dialex.presentation.nav.Connect
import com.dialex.presentation.nav.PersonaBuilder
import com.dialex.presentation.nav.Settings
import com.dialex.presentation.nav.Setup
import com.dialex.presentation.nav.Workspace
import com.dialex.presentation.nav.rememberAppBackStack
import com.dialex.presentation.settings.ProviderStatus
import com.dialex.presentation.settings.SettingsRoute
import com.dialex.presentation.settings.SettingsTab
import com.dialex.presentation.settings.personas.PersonaBuilderRoute
import com.dialex.presentation.setup.SetupRoute
import com.dialex.domain.usecase.SetupDiscussionWithAiUseCase
import com.dialex.presentation.workspace.AiSetupDialog
import com.dialex.presentation.workspace.WorkspaceSidebar
import com.dialex.presentation.workspace.WorkspaceHeader
import com.dialex.presentation.workspace.DesktopCompanionDialog
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import com.dialex.presentation.mobile.MobileNavigationShell
import com.dialex.presentation.chat.ChatTypographySettings
import com.dialex.presentation.chat.LocalChatTypographySettings
import com.dialex.presentation.chat.ChatDisplaySettings
import com.dialex.presentation.chat.LocalChatDisplaySettings
import com.dialex.ui.FeedbackDialog
import com.dialex.ui.SidebarResizeHandle
import com.dialex.service.isPlatformCliSupported
import com.dialex.service.checkPlatformCliAvailability
import com.dialex.service.checkPlatformCliLogins
import com.dialex.theme.AppTheme
import com.dialex.theme.ThemeMode
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

import com.dialex.data.datasource.LegalConsentLocalDataSource
import com.dialex.data.repository.LegalConsentRepositoryImpl
import com.dialex.domain.repository.LegalConsentRepository
import com.dialex.domain.usecase.GetLegalConsentUseCase
import com.dialex.domain.usecase.RecordLegalConsentUseCase
import com.dialex.presentation.legal.TermsConsentDialog
import com.dialex.domain.model.LegalConsent

/**
 * Root composable for the entire application.
 * Implements an adaptive two-pane workspace on wide screens
 * and single-pane on compact screens.
 */
@Composable
fun App(
    engineClient: EngineClient?,
    supportsCli: Boolean,
    supportsLocalEngine: Boolean,
    themeMode: ThemeMode,
    onThemeModeChange: (ThemeMode) -> Unit,
    connectionLabel: String,
    onSwitchConnection: () -> Unit,
    connectToLocalEngine: suspend () -> Unit,
    connectToRemote: suspend (url: String, username: String, password: String) -> Unit,
    recheckCli: suspend () -> List<ProviderStatus>,
    onExportMarkdown: (markdown: String, fileName: String) -> Unit,
    onToggleMaximizeWindow: (() -> Unit)? = null,
    extraSettingsTabLabel: String? = null,
    extraSettingsTabContent: (@Composable () -> Unit)? = null,
    modifier: Modifier = Modifier,
) {
    val initialRoute: AppRoute = if (engineClient == null) Connect else Workspace
    val backStack = rememberAppBackStack(initialRoute)

    // Wire local database & profile repositories
    val platformDb = remember { com.dialex.data.db.defaultPlatformDatabase() }
    val profileLocalDataSource = remember(platformDb) { ProfileLocalDataSource(platformDb) }
    val secureKeyDataSource = remember(platformDb) { SecureKeyDataSource(platformDb) }
    val localDatabaseSource = remember(platformDb) { LocalDatabaseSource(platformDb) }
    val legalConsentLocalDataSource = remember(platformDb) { LegalConsentLocalDataSource(platformDb) }

    val profileRepository: ProfileRepository = remember(profileLocalDataSource) {
        ProfileRepositoryImpl(profileLocalDataSource)
    }
    val apiKeyRepository: ApiKeyRepository = remember(secureKeyDataSource) {
        ApiKeyRepositoryImpl(secureKeyDataSource)
    }
    val localStoreRepository: LocalStoreRepository = remember(localDatabaseSource) {
        LocalStoreRepositoryImpl(localDatabaseSource)
    }
    val templateLocalDataSource = remember(platformDb) { TemplateLocalDataSource(platformDb) }
    val templateRepository: TemplateRepository = remember(templateLocalDataSource) {
        TemplateRepositoryImpl(templateLocalDataSource)
    }
    val legalConsentRepository: LegalConsentRepository = remember(legalConsentLocalDataSource) {
        LegalConsentRepositoryImpl(legalConsentLocalDataSource)
    }
    val getLegalConsentUseCase = remember(legalConsentRepository) {
        GetLegalConsentUseCase(legalConsentRepository)
    }
    val recordLegalConsentUseCase = remember(legalConsentRepository) {
        RecordLegalConsentUseCase(legalConsentRepository)
    }

    var legalConsent by remember { mutableStateOf<LegalConsent?>(null) }
    var isLegalConsentRequired by remember { mutableStateOf(false) }

    LaunchedEffect(legalConsentRepository) {
        getLegalConsentUseCase.observe().collect { consent ->
            legalConsent = consent
            isLegalConsentRequired = !consent.isAccepted || consent.termsVersion != LegalConsent.CURRENT_LEGAL_VERSION
        }
    }

    var activeProfile by remember { mutableStateOf<ConnectionProfile?>(null) }
    var isProfileLocked by remember { mutableStateOf(false) }

    LaunchedEffect(profileRepository) {
        profileRepository.ensureDefaultProfiles()
        profileRepository.observeActiveProfile().collect { profile ->
            activeProfile = profile
            if (profile != null) {
                isProfileLocked = profileRepository.isProfileLocked(profile.id)
            }
        }
    }

    val onConnectToLocalEngine: suspend () -> Unit = {
        profileRepository.ensureDefaultProfiles()
        profileRepository.setActiveProfile(ConnectionProfile.LOCAL_PROFILE_ID)
        connectToLocalEngine()
    }

    val onConnectToRemoteEngine: suspend (String, String, String) -> Unit = { url, username, password ->
        profileRepository.ensureDefaultProfiles()
        val currentRemote = profileRepository.getProfile(ConnectionProfile.REMOTE_PROFILE_ID)
        if (currentRemote != null) {
            profileRepository.saveProfile(currentRemote.copy(remoteUrl = url, remoteUsername = username))
        }
        profileRepository.setActiveProfile(ConnectionProfile.REMOTE_PROFILE_ID)
        connectToRemote(url, username, password)
    }

    // Wire data layer dependencies
    val dataSource = remember(engineClient) { engineClient?.let { EngineDataSource(it) } }
    val projectRepository: ProjectRepository? = remember(dataSource) { dataSource?.let { ProjectRepositoryImpl(it) } }
    val discussionRepository: DiscussionRepository? = remember(dataSource) { dataSource?.let { DiscussionRepositoryImpl(it) } }
    val personaRepository: PersonaRepository? = remember(dataSource) { dataSource?.let { PersonaRepositoryImpl(it) } }
    val settingsRepository: SettingsRepository? = remember(dataSource) { dataSource?.let { SettingsRepositoryImpl(it) } }
    val graphRepository: GraphRepository? = remember(dataSource) { dataSource?.let { GraphRepositoryImpl(it) } }
    val decompositionRepository: com.dialex.domain.repository.DecompositionRepository? = remember(dataSource) {
        dataSource?.let { com.dialex.data.repository.DecompositionRepositoryImpl(it) }
    }
    val decomposeProblemUseCase = remember(decompositionRepository) {
        decompositionRepository?.let { com.dialex.domain.usecase.DecomposeProblemUseCase(it) }
    }
    val benchmarkRepository: BenchmarkRepository? = remember(dataSource) {
        dataSource?.let { BenchmarkRepositoryImpl(it) }
    }
    val credenceRepository: com.dialex.domain.repository.CredenceRepository? = remember(dataSource) {
        dataSource?.let { com.dialex.data.repository.CredenceRepositoryImpl(it) }
    }
    val recalculateCredenceUseCase = remember(credenceRepository) {
        credenceRepository?.let { com.dialex.domain.usecase.RecalculateCredenceUseCase(it) }
    }

    // Live state of projects and discussions for the Sidebar
    var projects by remember { mutableStateOf<List<Project>>(emptyList()) }
    var discussions by remember { mutableStateOf<List<Discussion>>(emptyList()) }
    var selectedProjectId by remember { mutableStateOf<String?>(null) }
    var selectedDiscussionId by remember { mutableStateOf<String?>(null) }
    var showGlobalFeedbackDialog by remember { mutableStateOf(false) }
    var showCompanionDialog by remember { mutableStateOf(false) }
    var showAiSetupDialog by remember { mutableStateOf(false) }
    var aiSetupLoading by remember { mutableStateOf(false) }
    var aiSetupError by remember { mutableStateOf<String?>(null) }
    var configuredCompactionModel by remember { mutableStateOf("claude-haiku-4-5-20251001") }
    var catalogModelsMap by remember { mutableStateOf<Map<String, List<String>>>(emptyMap()) }
    var appApiKeys by remember { mutableStateOf(ApiKeys()) }
    var appCliCommands by remember { mutableStateOf(CliCommands()) }
    var appCliStatus by remember { mutableStateOf<Map<String, Boolean>>(emptyMap()) }
    var appCliLogins by remember { mutableStateOf<Map<String, Boolean>>(emptyMap()) }
    val coroutineScope = rememberCoroutineScope()

    fun refreshCliStatusAndLogins() {
        coroutineScope.launch {
            val platformAvail = if (isPlatformCliSupported()) checkPlatformCliAvailability() else emptyMap()
            val platformLogins = if (isPlatformCliSupported()) checkPlatformCliLogins() else emptyMap()

            val remoteAvail = runCatching { settingsRepository?.getCliStatus() }.getOrNull().orEmpty()
            val remoteLogins = runCatching { settingsRepository?.getCliLogins() }.getOrNull().orEmpty()

            val mergedAvail = platformAvail + remoteAvail
            val mergedLogins = platformLogins + remoteLogins

            appCliStatus = mergedAvail
            appCliLogins = mergedLogins
        }
    }

    LaunchedEffect(settingsRepository) {
        if (settingsRepository != null) {
            runCatching {
                val settings = settingsRepository.getSettings()
                if (settings.compactionModel.isNotBlank()) {
                    configuredCompactionModel = settings.compactionModel
                }
                appApiKeys = settings.apiKeys
                appCliCommands = settings.cliCommands
                catalogModelsMap = settingsRepository.getAvailableModels()
            }
            refreshCliStatusAndLogins()
        }
    }

    fun reloadSidebarData() {
        if (projectRepository != null && discussionRepository != null) {
            coroutineScope.launch {
                runCatching {
                    var projs = projectRepository.getProjects()
                    if (projs.none { it.name.equals("Ungrouped", ignoreCase = true) }) {
                        val ungrouped = projectRepository.createProject("Ungrouped")
                        projs = listOf(ungrouped) + projs
                    }
                    projects = projs
                    discussions = discussionRepository.getDiscussions()
                }
            }
        }
    }

    LaunchedEffect(engineClient) {
        if (engineClient == null) {
            if (backStack.current !is Connect) {
                backStack.set(listOf(Connect))
            }
        } else if (backStack.current is Connect) {
            backStack.set(listOf(Workspace))
        }
        reloadSidebarData()
    }

    // Reactively observe project list updates across the entire application
    LaunchedEffect(projectRepository) {
        val repo = projectRepository ?: return@LaunchedEffect
        repo.observeProjects().collect { updatedProjects ->
            if (updatedProjects.isNotEmpty()) {
                projects = updatedProjects
            }
        }
    }

    // Continuously keep discussions list fresh so sidebar status indicators (RUNNING, PAUSED, DONE) reflect live engine state
    LaunchedEffect(discussionRepository) {
        val repo = discussionRepository ?: return@LaunchedEffect
        while (isActive) {
            delay(1500)
            runCatching {
                val fresh = repo.getDiscussions()
                if (fresh != discussions) {
                    discussions = fresh
                }
            }
        }
    }

    // Keep active selections in sync with navigation routes
    LaunchedEffect(backStack.current) {
        when (val current = backStack.current) {
            is Chat -> {
                selectedDiscussionId = current.discussionId
                val disc = discussions.firstOrNull { it.id == current.discussionId }
                if (disc != null) selectedProjectId = disc.projectId
            }
            is Setup -> {
                if (current.discussionId != null) {
                    selectedDiscussionId = current.discussionId
                    val disc = discussions.firstOrNull { it.id == current.discussionId }
                    if (disc != null) selectedProjectId = disc.projectId
                } else {
                    selectedDiscussionId = null
                }
            }
            else -> {}
        }
        reloadSidebarData()
    }

    AppTheme(themeMode) {
        val chatTypographyState = remember { mutableStateOf(ChatTypographySettings.Default) }
        val chatDisplayState = remember { mutableStateOf(ChatDisplaySettings.Default) }

        CompositionLocalProvider(
            LocalChatTypographySettings provides chatTypographyState,
            LocalChatDisplaySettings provides chatDisplayState,
            LocalWindowMaximizeToggle provides onToggleMaximizeWindow
        ) {
        BoxWithConstraints(modifier = modifier.fillMaxSize()) {
            val isWideScreen = maxWidth >= 600.dp
            val isConnected = engineClient != null && backStack.current !is Connect
            var sidebarWidth by remember { mutableStateOf(280.dp) }
            var sidebarVisible by remember { mutableStateOf(true) }

            // Hardware back & predictive back gesture handling
            AppBackHandler(enabled = backStack.size > 1) {
                backStack.removeLast()
            }

            var hasAutoOpenedLastActive by remember { mutableStateOf(false) }

            // When opening the app, automatically open the last active / newest discussion
            LaunchedEffect(isConnected, discussions) {
                if (isConnected && !hasAutoOpenedLastActive && discussions.isNotEmpty()) {
                    hasAutoOpenedLastActive = true
                    val lastActive = discussions.lastOrNull { it.status == DiscussionStatus.RUNNING }
                        ?: discussions.lastOrNull()
                    if (lastActive != null) {
                        selectedDiscussionId = lastActive.id
                        selectedProjectId = lastActive.projectId
                        if (isWideScreen) {
                            if (lastActive.status == DiscussionStatus.DRAFT) {
                                backStack.set(listOf(Setup(lastActive.id)))
                            } else {
                                backStack.set(listOf(Chat(lastActive.id)))
                            }
                        } else {
                            if (lastActive.status == DiscussionStatus.DRAFT) {
                                backStack.set(listOf(Workspace, Setup(lastActive.id)))
                            } else {
                                backStack.set(listOf(Workspace, Chat(lastActive.id)))
                            }
                        }
                    }
                }
            }

            // Keep root navigation state adapted between compact single-pane and expanded two-pane
            LaunchedEffect(isWideScreen, isConnected) {
                if (isWideScreen && isConnected && backStack.entries.size == 1 && backStack.current is Workspace) {
                    val activeDiscId = selectedDiscussionId
                    if (activeDiscId != null) {
                        val disc = discussions.firstOrNull { it.id == activeDiscId }
                        if (disc?.status == DiscussionStatus.DRAFT) {
                            backStack.set(listOf(Setup(activeDiscId)))
                        } else {
                            backStack.set(listOf(Chat(activeDiscId)))
                        }
                    } else {
                        backStack.set(listOf(Setup(null)))
                    }
                } else if (!isWideScreen && isConnected && backStack.entries.size == 1 && backStack.current is Setup && (backStack.current as Setup).discussionId == null) {
                    backStack.set(listOf(Workspace))
                }
            }

            if (isConnected && isWideScreen) {
                val currentRoute = backStack.current
                if (currentRoute is Settings) {
                    // ── Full-Width Settings: Hides both side panels, giving settings full canvas breathing room ──
                    if (settingsRepository != null && personaRepository != null) {
                        SettingsRoute(
                            backStack = backStack,
                            settingsRepository = settingsRepository,
                            personaRepository = personaRepository,
                            themeMode = themeMode,
                            onThemeModeChange = onThemeModeChange,
                            connectionLabel = connectionLabel,
                            supportsLocalEngine = supportsLocalEngine,
                            onSwitchConnection = onSwitchConnection,
                            recheckCli = recheckCli,
                            initialTab = currentRoute.initialTab,
                            profileRepository = profileRepository,
                            apiKeyRepository = apiKeyRepository,
                            legalConsentRepository = legalConsentRepository,
                            extraTabLabel = extraSettingsTabLabel,
                            extraTabContent = extraSettingsTabContent,
                            isCompact = false,
                            modifier = Modifier.fillMaxSize()
                        )
                    }
                } else if (currentRoute is PersonaBuilder) {
                    // ── Full-Width Persona Builder ──
                    if (personaRepository != null) {
                        PersonaBuilderRoute(
                            backStack = backStack,
                            personaId = currentRoute.personaId,
                            personaRepository = personaRepository,
                            onBack = { backStack.removeLast() },
                            isCompact = false,
                        )
                    }
                } else if (currentRoute is BenchmarkArena) {
                    // ── Full-Width Benchmark Arena ──
                    if (benchmarkRepository != null) {
                        BenchmarkRoute(
                            benchmarkRepository = benchmarkRepository,
                            onBack = { backStack.removeLast() },
                            modifier = Modifier.fillMaxSize()
                        )
                    }
                } else if (currentRoute is Graph) {
                    // ── Full-Width Knowledge Graph ──
                    if (graphRepository != null) {
                        GraphRoute(
                            projectId = currentRoute.projectId,
                            graphRepository = graphRepository,
                            onBack = { backStack.removeLast() },
                            modifier = Modifier.fillMaxSize()
                        )
                    }
                } else {
                    // ── Two-Pane Layout (Master-Detail with Resizable Splitter) ──
                    Row(Modifier.fillMaxSize()) {
                    // Left Rail: Projects & Sessions Tree
                    if (sidebarVisible) {
                        WorkspaceSidebar(
                            projects = projects,
                            discussions = discussions,
                            selectedProjectId = selectedProjectId,
                            selectedDiscussionId = selectedDiscussionId,
                            onSelectProject = { projId: String ->
                                selectedProjectId = projId
                                selectedDiscussionId = null
                                backStack.set(listOf(Setup(discussionId = null, initialProjectId = projId)))
                            },
                            onSelectDiscussion = { discId: String ->
                                selectedDiscussionId = discId
                                val disc = discussions.firstOrNull { it.id == discId }
                                if (disc != null) selectedProjectId = disc.projectId
                                if (disc?.status == DiscussionStatus.DRAFT) {
                                    backStack.set(listOf(Setup(discId)))
                                } else {
                                    backStack.set(listOf(Chat(discId)))
                                }
                            },
                            onNewDiscussion = { projId: String? ->
                                val ungroupedId = projects.firstOrNull { it.name.equals("Ungrouped", ignoreCase = true) }?.id
                                val targetProjId = projId ?: selectedProjectId ?: ungroupedId ?: projects.firstOrNull()?.id
                                selectedProjectId = targetProjId
                                selectedDiscussionId = null
                                backStack.add(Setup(discussionId = null, initialProjectId = targetProjId))
                            },
                            onNewSocraticInterview = { projId: String? ->
                                val ungroupedId = projects.firstOrNull { it.name.equals("Ungrouped", ignoreCase = true) }?.id
                                val targetProjId = projId ?: selectedProjectId ?: ungroupedId ?: projects.firstOrNull()?.id
                                selectedProjectId = targetProjId
                                selectedDiscussionId = null
                                backStack.add(
                                    Setup(
                                        discussionId = null,
                                        initialProjectId = targetProjId,
                                        initialMode = com.dialex.domain.model.DiscussionMode.SOCRATIC_INTERVIEW
                                    )
                                )
                            },
                            onCopyDiscussionSettings = { sourceDisc ->
                                selectedProjectId = sourceDisc.projectId
                                selectedDiscussionId = null
                                backStack.add(Setup(discussionId = null, initialProjectId = sourceDisc.projectId, copyFromDiscussionId = sourceDisc.id))
                            },
                            onCreateProject = { name: String ->
                                coroutineScope.launch {
                                    try {
                                        val created = projectRepository?.createProject(name)
                                        if (created != null) {
                                            selectedProjectId = created.id
                                        }
                                        reloadSidebarData()
                                    } catch (e: Exception) {
                                        kolt.logutils.printLog("Failed to create project: ${e.message}", isError = true)
                                    }
                                }
                            },
                            onUpdateProject = { updatedProject: Project ->
                                coroutineScope.launch {
                                    try {
                                        projectRepository?.updateProject(updatedProject)
                                        reloadSidebarData()
                                    } catch (e: Exception) {
                                        kolt.logutils.printLog("Failed to update project: ${e.message}", isError = true)
                                    }
                                }
                            },
                            onDeleteProject = { projId: String ->
                                coroutineScope.launch {
                                    try {
                                        projectRepository?.deleteProject(projId)
                                        if (selectedProjectId == projId) {
                                            selectedProjectId = null
                                            selectedDiscussionId = null
                                            backStack.add(Setup(null))
                                        }
                                        reloadSidebarData()
                                    } catch (e: Exception) {
                                        kolt.logutils.printLog("Failed to delete project: ${e.message}", isError = true)
                                    }
                                }
                            },
                            onDeleteDiscussion = { discId: String ->
                                coroutineScope.launch {
                                    try {
                                        discussionRepository?.deleteDiscussion(discId)
                                        if (selectedDiscussionId == discId) {
                                            selectedDiscussionId = null
                                            selectedDiscussionId = null
                                            backStack.add(Setup(null))
                                        }
                                        reloadSidebarData()
                                    } catch (e: Exception) {
                                        kolt.logutils.printLog("Failed to delete discussion: ${e.message}", isError = true)
                                    }
                                }
                            },
                            onRenameDiscussion = { discId: String, newName: String ->
                                coroutineScope.launch {
                                    try {
                                        val disc = discussions.firstOrNull { it.id == discId }
                                        if (disc != null && newName.isNotBlank()) {
                                            val updated = discussionRepository?.updateDiscussion(disc.copy(name = newName.trim()))
                                            if (updated != null) {
                                                discussions = discussions.map { if (it.id == updated.id) updated else it }
                                            }
                                            reloadSidebarData()
                                        }
                                    } catch (e: Exception) {
                                        kolt.logutils.printLog("Failed to rename discussion $discId: ${e.message}", isError = true)
                                    }
                                }
                            },
                            onMoveDiscussionToProject = { discId: String, targetProjectId: String ->
                                coroutineScope.launch {
                                    try {
                                        val disc = discussions.firstOrNull { it.id == discId }
                                        if (disc != null && disc.projectId != targetProjectId) {
                                            discussionRepository?.updateDiscussion(disc.copy(projectId = targetProjectId))
                                            reloadSidebarData()
                                        }
                                    } catch (e: Exception) {
                                        kolt.logutils.printLog("Failed to move discussion to project: ${e.message}", isError = true)
                                    }
                                }
                            },
                            onOpenSettings = { backStack.add(Settings(SettingsTab.AiAgents)) },
                            onOpenAiSetup = {
                                aiSetupError = null
                                refreshCliStatusAndLogins()
                                showAiSetupDialog = true
                            },
                            onOpenPersonaBuilder = { backStack.add(Settings(SettingsTab.Personas)) },
                            onOpenKnowledgeGraph = { projId -> backStack.add(Graph(projId)) },
                            onOpenBenchmarkArena = { backStack.add(BenchmarkArena) },
                            onOpenAbout = { backStack.add(Settings(SettingsTab.About)) },
                            onSendFeedback = { showGlobalFeedbackDialog = true },
                            connectionLabel = connectionLabel,
                            userName = engineClient.currentUsername ?: if (supportsLocalEngine && connectionLabel.contains("Local", ignoreCase = true)) "Local User" else "Guest",
                            onSwitchConnection = onSwitchConnection,
                            onToggleSidebar = { sidebarVisible = false },
                            cliStatus = appCliStatus,
                            cliLogins = appCliLogins,
                            onRecheckCli = { refreshCliStatusAndLogins() },
                            isCompact = false,
                            modifier = Modifier.width(sidebarWidth)
                        )

                        // Clear resizable splitter with visual indication
                        SidebarResizeHandle(
                            onResizeDelta = { deltaDp ->
                                val newWidth = (sidebarWidth + deltaDp).coerceIn(200.dp, 480.dp)
                                sidebarWidth = newWidth
                            }
                        )
                    }

                    // Right Canvas: Active View (Setup / Chat / Settings / PersonaBuilder)
                    Column(Modifier.weight(1f).fillMaxHeight()) {
                        val currentDisc = discussions.firstOrNull { it.id == selectedDiscussionId }
                        val currentProj = projects.firstOrNull { it.id == (selectedProjectId ?: currentDisc?.projectId) }
                        var artifactsPaneOpen by remember { mutableStateOf(false) }
                        var summaryDialogOpen by remember { mutableStateOf(false) }
                        var credenceDrawerOpen by remember { mutableStateOf(false) }
                        var lastSeenArtifactsCount by remember(selectedDiscussionId) { mutableStateOf(0) }

                        // Discussion chat search states
                        var chatSearchQuery by remember(selectedDiscussionId) { mutableStateOf("") }
                        var isChatSearchActive by remember(selectedDiscussionId) { mutableStateOf(false) }
                        var chatSearchMatchCount by remember(selectedDiscussionId) { mutableStateOf(0) }
                        var currentChatSearchMatchIndex by remember(selectedDiscussionId) { mutableStateOf(0) }
                        var chatSearchNextTrigger by remember { mutableStateOf(0) }
                        var chatSearchPrevTrigger by remember { mutableStateOf(0) }

                        val totalArtifactsCount = remember(currentDisc) {
                            if (currentDisc == null) 0
                            else if (currentDisc.artifacts.isNotEmpty()) currentDisc.artifacts.size
                            else listOfNotNull(
                                currentDisc.deliverable,
                                currentDisc.summary,
                                if (currentDisc.transcript.isNotEmpty()) currentDisc.transcript else null
                            ).size
                        }

                        val hasUnreadArtifacts = totalArtifactsCount > lastSeenArtifactsCount && !artifactsPaneOpen

                        // Breadcrumb Header (Shown for Chat; Setup has its own dedicated top bar with Start Discussion CTA)
                        if (backStack.current is Chat) {
                            val typSettings by chatTypographyState
                            WorkspaceHeader(
                                projectName = currentProj?.name,
                                discussionName = currentDisc?.name,
                                status = currentDisc?.status,
                                readingSettings = typSettings,
                                onReadingSettingsChange = { chatTypographyState.value = it },
                                onToggleSummary = { summaryDialogOpen = !summaryDialogOpen },
                                onOpenArtifacts = {
                                    val willOpen = !artifactsPaneOpen
                                    artifactsPaneOpen = willOpen
                                    if (willOpen) {
                                        lastSeenArtifactsCount = totalArtifactsCount
                                    }
                                },
                                artifactsCount = totalArtifactsCount,
                                hasUnreadArtifacts = hasUnreadArtifacts,
                                onRenameDiscussion = if (currentDisc != null) {
                                    { newName ->
                                        if (newName.isNotBlank() && newName != currentDisc.name) {
                                            coroutineScope.launch {
                                                try {
                                                    val updated = discussionRepository?.updateDiscussion(currentDisc.copy(name = newName.trim()))
                                                    if (updated != null) {
                                                        discussions = discussions.map { if (it.id == updated.id) updated else it }
                                                    }
                                                } catch (e: Exception) {
                                                    kolt.logutils.printLog("Failed to rename discussion ${currentDisc.id}: ${e.message}", isError = true)
                                                }
                                            }
                                        }
                                    }
                                } else null,
                                onEditSetup = if (selectedDiscussionId != null) {
                                    { backStack.add(Setup(selectedDiscussionId)) }
                                } else null,
                                onExportMarkdown = if (currentDisc != null) {
                                    {
                                        val md = currentDisc.toMarkdown()
                                        val fn = currentDisc.exportFileName()
                                        onExportMarkdown(md, fn)
                                    }
                                } else null,
                                onExportMemo = if (currentDisc != null) {
                                    {
                                        val html = currentDisc.toExecutiveMemorandumHtml(currentProj?.name)
                                        val fn = currentDisc.exportMemoFileName()
                                        onExportMarkdown(html, fn)
                                    }
                                } else null,
                                onPairMobile = { showCompanionDialog = true },
                                onOpenSettings = {
                                    backStack.add(Settings(SettingsTab.Compaction))
                                },
                                onToggleSidebar = if (!sidebarVisible) { { sidebarVisible = true } } else null,
                                searchQuery = chatSearchQuery,
                                onSearchQueryChange = { chatSearchQuery = it },
                                isSearchActive = isChatSearchActive,
                                onToggleSearch = { active ->
                                    isChatSearchActive = active
                                    if (!active) chatSearchQuery = ""
                                },
                                searchMatchCount = chatSearchMatchCount,
                                currentSearchMatchIndex = currentChatSearchMatchIndex,
                                onNextSearchMatch = { chatSearchNextTrigger++ },
                                onPrevSearchMatch = { chatSearchPrevTrigger++ },
                                credenceLedger = currentDisc?.credenceLedger,
                                onOpenCredenceDrawer = { credenceDrawerOpen = true }
                            )

                            if (credenceDrawerOpen && currentDisc?.credenceLedger != null) {
                                com.dialex.presentation.chat.CredenceDrawer(
                                    ledger = currentDisc.credenceLedger,
                                    onDismiss = { credenceDrawerOpen = false },
                                    onRecalculate = {
                                        val disc = currentDisc
                                        if (disc != null && recalculateCredenceUseCase != null) {
                                            coroutineScope.launch {
                                                try {
                                                    val updatedLedger = recalculateCredenceUseCase(disc.id).getOrThrow()
                                                    val updatedDisc = disc.copy(credenceLedger = updatedLedger)
                                                    discussions = discussions.map { if (it.id == updatedDisc.id) updatedDisc else it }
                                                } catch (e: Exception) {
                                                    kolt.logutils.printLog("Failed to recalculate credence: ${e.message}", isError = true)
                                                }
                                            }
                                        }
                                    }
                                )
                            }
                        }

                        Box(Modifier.weight(1f).fillMaxWidth()) {
                            AnimatedContent(
                                targetState = backStack.current,
                                transitionSpec = { fadeIn() togetherWith fadeOut() },
                                label = "WorkspaceContent"
                            ) { targetRoute ->
                                when (targetRoute) {
                                    is Setup -> {
                                        if (projectRepository != null && discussionRepository != null && settingsRepository != null) {
                                            SetupRoute(
                                                backStack = backStack,
                                                discussionId = targetRoute.discussionId,
                                                initialProjectId = targetRoute.initialProjectId,
                                                copyFromDiscussionId = targetRoute.copyFromDiscussionId,
                                                initialMode = targetRoute.initialMode,
                                                supportsCli = supportsCli,
                                                projectRepository = projectRepository,
                                                discussionRepository = discussionRepository,
                                                settingsRepository = settingsRepository,
                                                templateRepository = templateRepository,
                                                decompositionUseCase = decomposeProblemUseCase,
                                                onToggleSidebar = if (!sidebarVisible) { { sidebarVisible = true } } else null,
                                                onBack = {
                                                    if (backStack.size > 1) {
                                                        backStack.removeLast()
                                                    } else if (targetRoute.discussionId != null) {
                                                        backStack.replaceTop(Chat(targetRoute.discussionId))
                                                    }
                                                },
                                                isCompact = false
                                            )
                                        }
                                    }
                                    is Chat -> {
                                        if (discussionRepository != null) {
                                            ChatRoute(
                                                backStack = backStack,
                                                discussionId = targetRoute.discussionId,
                                                tokenBudget = 1_000_000,
                                                discussionRepository = discussionRepository,
                                                onExportMarkdown = onExportMarkdown,
                                                projectName = currentProj?.name,
                                                artifactsPaneOpen = artifactsPaneOpen,
                                                onToggleArtifacts = { artifactsPaneOpen = !artifactsPaneOpen },
                                                summaryDialogOpen = summaryDialogOpen,
                                                onToggleSummary = { summaryDialogOpen = !summaryDialogOpen },
                                                onDiscussionUpdated = { updatedDisc ->
                                                    discussions = discussions.map { if (it.id == updatedDisc.id) updatedDisc else it }
                                                },
                                                searchQuery = chatSearchQuery,
                                                onSearchQueryChange = { chatSearchQuery = it },
                                                isSearchActive = isChatSearchActive,
                                                onToggleSearch = { active ->
                                                    isChatSearchActive = active
                                                    if (!active) chatSearchQuery = ""
                                                },
                                                searchNextTrigger = chatSearchNextTrigger,
                                                searchPrevTrigger = chatSearchPrevTrigger,
                                                onSearchMatchesChanged = { count, index ->
                                                    chatSearchMatchCount = count
                                                    currentChatSearchMatchIndex = index
                                                },
                                                isCompact = false,
                                                recalculateCredenceUseCase = recalculateCredenceUseCase
                                            )
                                        }
                                    }
                                    is Settings -> {
                                        if (settingsRepository != null && personaRepository != null) {
                                            SettingsRoute(
                                                backStack = backStack,
                                                settingsRepository = settingsRepository,
                                                personaRepository = personaRepository,
                                                themeMode = themeMode,
                                                onThemeModeChange = onThemeModeChange,
                                                connectionLabel = connectionLabel,
                                                supportsLocalEngine = supportsLocalEngine,
                                                onSwitchConnection = onSwitchConnection,
                                                recheckCli = recheckCli,
                                                initialTab = targetRoute.initialTab,
                                                profileRepository = profileRepository,
                                                apiKeyRepository = apiKeyRepository,
                                                legalConsentRepository = legalConsentRepository,
                                                extraTabLabel = extraSettingsTabLabel,
                                                extraTabContent = extraSettingsTabContent,
                                                isCompact = false,
                                            )
                                        }
                                    }
                                    is PersonaBuilder -> {
                                        if (personaRepository != null) {
                                            PersonaBuilderRoute(
                                                backStack = backStack,
                                                personaId = targetRoute.personaId,
                                                personaRepository = personaRepository,
                                                onBack = { backStack.removeLast() },
                                                isCompact = false,
                                            )
                                        }
                                    }
                                    is Workspace -> {
                                        if (selectedDiscussionId != null) {
                                            val disc = discussions.firstOrNull { it.id == selectedDiscussionId }
                                            if (disc?.status == DiscussionStatus.DRAFT) {
                                                if (projectRepository != null && discussionRepository != null && settingsRepository != null) {
                                                    SetupRoute(
                                                        backStack = backStack,
                                                        discussionId = selectedDiscussionId,
                                                        initialProjectId = selectedProjectId,
                                                        supportsCli = supportsCli,
                                                        projectRepository = projectRepository,
                                                        discussionRepository = discussionRepository,
                                                        settingsRepository = settingsRepository,
                                                        templateRepository = templateRepository,
                                                        decompositionUseCase = decomposeProblemUseCase,
                                                        onToggleSidebar = if (!sidebarVisible) { { sidebarVisible = true } } else null,
                                                        isCompact = false
                                                    )
                                                }
                                            } else if (discussionRepository != null) {
                                                ChatRoute(
                                                    backStack = backStack,
                                                    discussionId = selectedDiscussionId!!,
                                                    tokenBudget = 1_000_000,
                                                    discussionRepository = discussionRepository,
                                                    onExportMarkdown = onExportMarkdown,
                                                    onDiscussionUpdated = { updatedDisc ->
                                                        discussions = discussions.map { if (it.id == updatedDisc.id) updatedDisc else it }
                                                    },
                                                    recalculateCredenceUseCase = recalculateCredenceUseCase,
                                                    isCompact = false
                                                )
                                            }
                                        } else if (projectRepository != null && discussionRepository != null && settingsRepository != null) {
                                            SetupRoute(
                                                backStack = backStack,
                                                discussionId = null,
                                                initialProjectId = selectedProjectId,
                                                supportsCli = supportsCli,
                                                projectRepository = projectRepository,
                                                discussionRepository = discussionRepository,
                                                settingsRepository = settingsRepository,
                                                templateRepository = templateRepository,
                                                decompositionUseCase = decomposeProblemUseCase,
                                                onToggleSidebar = if (!sidebarVisible) { { sidebarVisible = true } } else null,
                                                isCompact = false
                                            )
                                        }
                                    }
                                    is Connect -> {}
                                    is Graph -> {
                                        if (graphRepository != null) {
                                            GraphRoute(
                                                projectId = targetRoute.projectId,
                                                graphRepository = graphRepository,
                                                onBack = { backStack.removeLast() },
                                                modifier = Modifier.fillMaxSize()
                                            )
                                        }
                                    }
                                    is BenchmarkArena -> {
                                        if (benchmarkRepository != null) {
                                            BenchmarkRoute(
                                                benchmarkRepository = benchmarkRepository,
                                                onBack = { backStack.removeLast() },
                                                modifier = Modifier.fillMaxSize()
                                            )
                                        }
                                    }
                                    is LicensingDedication -> {
                                        LicensingRoute(
                                            backStack = backStack,
                                            onBack = { backStack.removeLast() },
                                            modifier = Modifier.fillMaxSize()
                                        )
                                    }
                                }
                            }
                        }
                    }
                }
            }
        } else {
                // ── Single-Pane Layout (Mobile Compact Drill-Down) ───────────
                Box(Modifier.fillMaxSize()) {
                    AnimatedContent(
                        targetState = backStack.current,
                        transitionSpec = {
                            if (backStack.isNavigatingBack) {
                                slideInHorizontally { width -> -width } + fadeIn() togetherWith
                                slideOutHorizontally { width -> width } + fadeOut()
                            } else {
                                slideInHorizontally { width -> width } + fadeIn() togetherWith
                                slideOutHorizontally { width -> -width } + fadeOut()
                            }
                        },
                        label = "AppNavigation"
                    ) { targetRoute ->
                        when (targetRoute) {
                            is Connect -> {
                                ConnectRoute(
                                    backStack = backStack,
                                    connectToLocalEngine = onConnectToLocalEngine,
                                    connectToRemote = onConnectToRemoteEngine,
                                    supportsLocalEngine = supportsLocalEngine,
                                )
                            }
                            is Setup -> {
                                if (projectRepository != null && discussionRepository != null && settingsRepository != null) {
                                    SetupRoute(
                                        backStack = backStack,
                                        discussionId = targetRoute.discussionId,
                                        initialProjectId = targetRoute.initialProjectId,
                                        copyFromDiscussionId = targetRoute.copyFromDiscussionId,
                                        initialMode = targetRoute.initialMode,
                                        supportsCli = supportsCli,
                                        projectRepository = projectRepository,
                                        discussionRepository = discussionRepository,
                                        settingsRepository = settingsRepository,
                                        templateRepository = templateRepository,
                                        decompositionUseCase = decomposeProblemUseCase,
                                        onBack = { backStack.removeLast() },
                                        isCompact = true,
                                    )
                                }
                            }
                            is Chat -> {
                                if (discussionRepository != null) {
                                    val currentDisc = discussions.firstOrNull { it.id == targetRoute.discussionId }
                                    val currentProj = projects.firstOrNull { it.id == (selectedProjectId ?: currentDisc?.projectId) }
                                    ChatRoute(
                                        backStack = backStack,
                                        discussionId = targetRoute.discussionId,
                                        tokenBudget = 1_000_000,
                                        discussionRepository = discussionRepository,
                                        onExportMarkdown = onExportMarkdown,
                                        onBack = { backStack.removeLast() },
                                        projectName = currentProj?.name,
                                        onEditSetup = { backStack.add(Setup(targetRoute.discussionId)) },
                                        onOpenSettings = { backStack.add(Settings(SettingsTab.Compaction)) },
                                        onDiscussionUpdated = { updatedDisc ->
                                            discussions = discussions.map { if (it.id == updatedDisc.id) updatedDisc else it }
                                        },
                                        isCompact = true,
                                        recalculateCredenceUseCase = recalculateCredenceUseCase,
                                    )
                                }
                            }
                            is Settings -> {
                                if (settingsRepository != null && personaRepository != null) {
                                    SettingsRoute(
                                        backStack = backStack,
                                        settingsRepository = settingsRepository,
                                        personaRepository = personaRepository,
                                        themeMode = themeMode,
                                        onThemeModeChange = onThemeModeChange,
                                        connectionLabel = connectionLabel,
                                        supportsLocalEngine = supportsLocalEngine,
                                        onSwitchConnection = onSwitchConnection,
                                        recheckCli = recheckCli,
                                        initialTab = targetRoute.initialTab,
                                        profileRepository = profileRepository,
                                        apiKeyRepository = apiKeyRepository,
                                        legalConsentRepository = legalConsentRepository,
                                        extraTabLabel = extraSettingsTabLabel,
                                        extraTabContent = extraSettingsTabContent,
                                        isCompact = true,
                                    )
                                }
                            }
                            is PersonaBuilder -> {
                                if (personaRepository != null) {
                                    PersonaBuilderRoute(
                                        backStack = backStack,
                                        personaId = targetRoute.personaId,
                                        personaRepository = personaRepository,
                                        onBack = { backStack.removeLast() },
                                        isCompact = true,
                                    )
                                }
                            }
                            is Graph -> {
                                if (graphRepository != null) {
                                    GraphRoute(
                                        projectId = targetRoute.projectId,
                                        graphRepository = graphRepository,
                                        onBack = { backStack.removeLast() },
                                        modifier = Modifier.fillMaxSize()
                                    )
                                }
                            }
                            is BenchmarkArena -> {
                                if (benchmarkRepository != null) {
                                    BenchmarkRoute(
                                        benchmarkRepository = benchmarkRepository,
                                        onBack = { backStack.removeLast() },
                                        modifier = Modifier.fillMaxSize()
                                    )
                                }
                            }
                            is LicensingDedication -> {
                                LicensingRoute(
                                    backStack = backStack,
                                    onBack = { backStack.removeLast() },
                                    modifier = Modifier.fillMaxSize()
                                )
                            }
                            is Workspace -> {
                                MobileNavigationShell(
                                    discussions = discussions,
                                    activeProfile = activeProfile,
                                    profileRepository = profileRepository,
                                    apiKeyRepository = apiKeyRepository,
                                    themeMode = themeMode,
                                    onThemeModeChange = { mode ->
                                        onThemeModeChange(mode)
                                    },
                                    projects = projects,
                                    personaRepository = personaRepository,
                                    onOpenBenchmarkArena = {
                                        backStack.add(BenchmarkArena)
                                    },
                                    onOpenKnowledgeGraph = { pId ->
                                        backStack.add(Graph(pId))
                                    },
                                    onOpenSocraticInterview = {
                                        val ungroupedId = projects.firstOrNull { it.name.equals("Ungrouped", ignoreCase = true) }?.id
                                        val targetProjId = selectedProjectId ?: ungroupedId ?: projects.firstOrNull()?.id
                                        selectedProjectId = targetProjId
                                        selectedDiscussionId = null
                                        backStack.add(Setup(discussionId = null, initialProjectId = targetProjId, initialMode = com.dialex.domain.model.DiscussionMode.SOCRATIC_INTERVIEW))
                                    },
                                    onCreateProject = { name ->
                                        coroutineScope.launch {
                                            projectRepository?.createProject(name)
                                            reloadSidebarData()
                                        }
                                    },
                                    onOpenPersonaBuilder = { pId ->
                                        backStack.add(PersonaBuilder(pId))
                                    },
                                    onSelectDiscussion = { discId ->
                                        selectedDiscussionId = discId
                                        val disc = discussions.firstOrNull { it.id == discId }
                                        if (disc != null) selectedProjectId = disc.projectId
                                        if (disc?.status == DiscussionStatus.DRAFT) {
                                            backStack.add(Setup(discId))
                                        } else {
                                            backStack.add(Chat(discId))
                                        }
                                    },
                                    onLaunchPreset = { preset ->
                                        coroutineScope.launch {
                                            val ungroupedId = projects.firstOrNull { it.name.equals("Ungrouped", ignoreCase = true) }?.id
                                            val targetProjId = selectedProjectId ?: ungroupedId ?: projects.firstOrNull()?.id
                                            if (targetProjId != null && discussionRepository != null) {
                                                val created = discussionRepository.createDiscussion(targetProjId, preset.title, preset.toDebateConfig())
                                                selectedDiscussionId = created.id
                                                selectedProjectId = targetProjId
                                                // Auto mode does not wait for user input — launch deliberation immediately
                                                runCatching { discussionRepository.startDiscussion(created.id) }
                                                reloadSidebarData()
                                                backStack.add(Chat(created.id))
                                            }
                                        }
                                    },
                                    onNewDilemma = {
                                        val ungroupedId = projects.firstOrNull { it.name.equals("Ungrouped", ignoreCase = true) }?.id
                                        val targetProjId = selectedProjectId ?: ungroupedId ?: projects.firstOrNull()?.id
                                        selectedProjectId = targetProjId
                                        selectedDiscussionId = null
                                        backStack.add(Setup(discussionId = null, initialProjectId = targetProjId))
                                    },
                                    onExportMarkdown = onExportMarkdown,
                                    onSwitchToLocal = {
                                        coroutineScope.launch {
                                            onConnectToLocalEngine()
                                        }
                                    },
                                    onSwitchToRemote = {
                                        backStack.add(Connect)
                                    },
                                    onScanQr = {
                                        backStack.add(Connect)
                                    },
                                    modifier = Modifier.fillMaxSize()
                                )
                            }
                        }
                    }
                }
            }
        }
    }

    if (isProfileLocked && activeProfile != null) {
        ProfileLockDialog(
            profile = activeProfile!!,
            onUnlockWithPin = { pin ->
                val ok = profileRepository.unlockProfileWithPin(activeProfile!!.id, pin)
                if (ok) {
                    isProfileLocked = false
                }
                ok
            },
            onUnlockWithBiometric = {
                profileRepository.unlockProfileWithBiometric(activeProfile!!.id)
                isProfileLocked = false
            },
            onSwitchProfile = {
                onSwitchConnection()
            }
        )
    }

    if (showGlobalFeedbackDialog) {
        val chatDisplayState = LocalChatDisplaySettings.current
        FeedbackDialog(
            initialBotToken = chatDisplayState.value.telegramBotToken,
            initialChatId = chatDisplayState.value.telegramChatId,
            sessionTranscript = null,
            onDismiss = { showGlobalFeedbackDialog = false },
            onSuccess = {
                showGlobalFeedbackDialog = false
            }
        )
    }

    if (showCompanionDialog && engineClient != null) {
        val clipboard = LocalClipboardManager.current
        DesktopCompanionDialog(
            serverUrl = engineClient.activeBaseUrl,
            authToken = engineClient.token ?: "",
            username = engineClient.currentUsername ?: if (supportsLocalEngine && connectionLabel.contains("Local", ignoreCase = true)) "Local User" else "admin",
            isTsnetEnabled = false,
            tailscaleUrl = engineClient.fallbackUrl,
            onDismiss = { showCompanionDialog = false },
            onToggleTsnet = { _, _ -> },
            onCopyPairingCode = { code ->
                clipboard.setText(AnnotatedString(code))
            }
        )
    }

    if (showAiSetupDialog) {
        AiSetupDialog(
            projects = projects,
            initialProjectId = selectedProjectId,
            defaultModel = configuredCompactionModel.ifBlank { "claude-haiku-4-5-20251001" },
            availableModels = catalogModelsMap,
            apiKeys = appApiKeys,
            cliCommands = appCliCommands,
            cliStatus = appCliStatus,
            cliLogins = appCliLogins,
            onDismiss = {
                showAiSetupDialog = false
            },
            onArchitect = { prompt, projId, model, numAgents ->
                val discRepo = discussionRepository ?: error("Discussion repository unavailable")
                val useCase = SetupDiscussionWithAiUseCase(discRepo)
                val created = useCase(
                    prompt = prompt,
                    projectId = projId,
                    model = model,
                    autoStart = false,
                    numAgents = numAgents,
                    selectedAgents = null
                )
                reloadSidebarData()
                selectedProjectId = created.projectId
                selectedDiscussionId = created.id
                created
            },
            onConfirmLaunch = { disc ->
                val discRepo = discussionRepository ?: error("Discussion repository unavailable")
                showAiSetupDialog = false
                reloadSidebarData()
                selectedProjectId = disc.projectId
                selectedDiscussionId = disc.id
                runCatching { discRepo.startDiscussion(disc.id) }
                backStack.set(listOf(Chat(disc.id)))
            },
            onViewFullSetup = { disc ->
                showAiSetupDialog = false
                reloadSidebarData()
                selectedProjectId = disc.projectId
                selectedDiscussionId = disc.id
                backStack.set(listOf(Setup(disc.id)))
            }
        )
    }

    if (isLegalConsentRequired) {
        TermsConsentDialog(
            onAccept = {
                coroutineScope.launch {
                    recordLegalConsentUseCase()
                    isLegalConsentRequired = false
                }
            },
            onDecline = {
                // User declined consent
            }
        )
    }
    } // end CompositionLocalProvider
}
