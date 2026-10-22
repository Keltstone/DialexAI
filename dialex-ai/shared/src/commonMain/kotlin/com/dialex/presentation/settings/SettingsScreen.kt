@file:Suppress("DEPRECATION")

package com.dialex.presentation.settings

import kotlin.math.roundToInt
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import com.dialex.ui.GradientButton
import com.dialex.logging.AppLogStore
import com.dialex.logging.AppLogLevel
import com.dialex.logging.AppLogEntry
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.text.style.TextOverflow
import com.dialex.domain.model.ProfileLockConfig
import com.dialex.domain.model.ProfileType
import com.dialex.presentation.profile.ProfileLockSetupDialog
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Fingerprint
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.LockOpen
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Check
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.FileDownload
import androidx.compose.material.icons.outlined.FileOpen
import androidx.compose.material.icons.outlined.FileUpload
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material.icons.outlined.Language
import androidx.compose.material.icons.outlined.Search
import androidx.compose.material.icons.outlined.SmartToy
import androidx.compose.material.icons.outlined.Speed
import androidx.compose.material.icons.outlined.Tune
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.input.key.*
import androidx.compose.ui.text.font.FontFamily
import com.dialex.presentation.setup.FilePicker
import com.dialex.ui.AboutDialog
import com.dialex.ui.DialexLogoView
import com.dialex.ui.PersonaIconView
import com.dialex.ui.FeedbackDialog
import androidx.compose.material.icons.outlined.BugReport
import androidx.compose.material.icons.outlined.Terminal
import androidx.compose.material.icons.outlined.Download
import androidx.compose.material.icons.automirrored.outlined.Login
import com.dialex.presentation.chat.LocalChatDisplaySettings
import com.dialex.ui.ThemedTooltipBox
import com.dialex.ui.SubtleTextArea
import com.dialex.ui.CliManagementDialog
import com.dialex.service.SupportedCliTools
import com.dialex.service.launchCliLoginTerminal
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.dialex.model.ApiKeys
import com.dialex.model.CliCommands
import com.dialex.model.CompactionSettings
import com.dialex.model.PredefinedPersona
import com.dialex.model.brandName
import com.dialex.theme.LocalCcColors
import com.dialex.theme.ThemeMode
import com.dialex.model.Provider
import com.dialex.model.DebatePolicy
import com.dialex.model.UserInterventionPolicy
import com.dialex.model.ModelSource
import com.dialex.model.InterventionStyle
import com.dialex.model.DeliverableFormat
import com.dialex.ui.AestheticSlider
import com.dialex.ui.AestheticSwitch
import kolt.utils.state.AsyncState
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

enum class SettingsCategory(
    val title: String,
    val icon: ImageVector,
    val tabs: List<SettingsTab>
) {
    General(
        title = "General",
        icon = Icons.Outlined.Tune,
        tabs = listOf(SettingsTab.Appearance, SettingsTab.Connection)
    ),
    AiAndPersonas(
        title = "AI & Personas",
        icon = Icons.Outlined.SmartToy,
        tabs = listOf(SettingsTab.AiAgents, SettingsTab.Personas, SettingsTab.MasterInstructions, SettingsTab.DebatePolicy)
    ),
    EngineAndLimits(
        title = "Engine & Limits",
        icon = Icons.Outlined.Speed,
        tabs = listOf(SettingsTab.Compaction, SettingsTab.Limits, SettingsTab.Connection)
    ),
    AboutAndLegal(
        title = "About & Legal",
        icon = Icons.Outlined.Info,
        tabs = listOf(SettingsTab.About)
    );

    companion object {
        fun fromTab(tab: SettingsTab): SettingsCategory =
            entries.firstOrNull { it.tabs.contains(tab) } ?: General
    }
}

private fun getTabTitleAndSubtitle(
    selectedTab: SettingsTab,
    isExtraTabSelected: Boolean,
    extraTabLabel: String?
): Pair<String, String> {
    if (isExtraTabSelected) {
        return (extraTabLabel ?: "Servers") to "Manage remote engine server connections and fleet configurations."
    }
    return when (selectedTab) {
        SettingsTab.Appearance -> "General" to "Configure interface theme, appearance, and display preferences."
        SettingsTab.Security -> "Security & Profiles" to "Manage profile lock credentials, biometric challenges, and active connection profiles."
        SettingsTab.AiAgents -> "AI Agents & Models" to "Configure provider credentials, API keys, and CLI runner execution."
        SettingsTab.Personas -> "Personas" to "Manage custom agent personality definitions, export or import persona catalogs."
        SettingsTab.MasterInstructions -> "Master Instructions" to "Global guidelines and core principles injected into every participant agent prompt."
        SettingsTab.DebatePolicy -> "Autopilot & Debate Defaults" to "Configure global debate execution policies, autonomous autopilot defaults, and moderation triggers."
        SettingsTab.Compaction -> "Context Compaction" to "Control conversation summarization triggers and compaction models."
        SettingsTab.Limits -> "Limits & Budget" to "Configure maximum discussion token thresholds and safety boundaries."
        SettingsTab.Connection -> "Engine Connection" to "Configure engine server endpoints and local or remote connection."
        SettingsTab.Logs -> "API Inspector & Server Logs" to "Chucker-style transaction inspector for Agent, CLI, and Engine calls, alongside server runtime logs."
        SettingsTab.About -> "About & Legal" to "Product architecture, data sovereignty charter, PolyForm Noncommercial license, and terms of service."
    }
}

@Composable
fun SettingCard(
    modifier: Modifier = Modifier,
    content: @Composable ColumnScope.() -> Unit
) {
    val cc = LocalCcColors.current
    Surface(
        shape = RoundedCornerShape(12.dp),
        color = cc.panel,
        border = BorderStroke(1.dp, cc.border.copy(alpha = 0.6f)),
        modifier = modifier.fillMaxWidth()
    ) {
        Column(modifier = Modifier.fillMaxWidth()) {
            content()
        }
    }
}

@Composable
fun SettingRow(
    title: String,
    description: String? = null,
    modifier: Modifier = Modifier,
    control: @Composable () -> Unit = {}
) {
    val cc = LocalCcColors.current
    Row(
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 13.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween
    ) {
        Column(
            modifier = Modifier.weight(1f).padding(end = 16.dp),
            verticalArrangement = Arrangement.spacedBy(2.dp)
        ) {
            Text(
                title,
                style = MaterialTheme.typography.bodyMedium.copy(
                    fontWeight = FontWeight.SemiBold,
                    fontSize = 13.5.sp
                ),
                color = cc.textPrimary
            )
            if (description != null) {
                Text(
                    description,
                    style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                    color = cc.textMuted
                )
            }
        }
        control()
    }
}

@Composable
private fun SettingsNavPill(
    label: String,
    isSelected: Boolean,
    onClick: () -> Unit
) {
    val cc = LocalCcColors.current
    val bg = if (isSelected) (if (cc.isDark) Color(0xFF262632) else Color(0xFFE5E7EB)) else Color.Transparent
    val textColor = if (isSelected) cc.textPrimary else cc.textMuted
    val fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal

    Surface(
        shape = RoundedCornerShape(8.dp),
        color = bg,
        modifier = Modifier
            .fillMaxWidth()
            .height(34.dp)
            .clip(RoundedCornerShape(8.dp))
            .clickable(onClick = onClick)
    ) {
        Row(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 12.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                label,
                style = MaterialTheme.typography.bodyMedium.copy(
                    fontSize = 13.sp,
                    fontWeight = fontWeight
                ),
                color = textColor
            )
        }
    }
}

@Composable
fun <T> SubtleSegmentedControl(
    options: List<Pair<T, String>>,
    selected: T,
    onSelect: (T) -> Unit,
    modifier: Modifier = Modifier,
) {
    val cc = LocalCcColors.current
    Row(
        modifier = modifier
            .clip(RoundedCornerShape(8.dp))
            .background(cc.panelAlt)
            .border(BorderStroke(0.75.dp, cc.border.copy(alpha = 0.45f)), RoundedCornerShape(8.dp))
            .padding(3.dp),
        horizontalArrangement = Arrangement.spacedBy(3.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        options.forEach { (key, label) ->
            val isSelected = key == selected
            Surface(
                shape = RoundedCornerShape(6.dp),
                color = if (isSelected) cc.panel else Color.Transparent,
                border = if (isSelected) BorderStroke(0.75.dp, cc.border.copy(alpha = 0.7f)) else null,
                modifier = Modifier
                    .clip(RoundedCornerShape(6.dp))
                    .clickable { onSelect(key) }
            ) {
                Text(
                    label,
                    style = MaterialTheme.typography.labelMedium.copy(
                        fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Medium,
                        fontSize = 12.5.sp
                    ),
                    color = if (isSelected) cc.textPrimary else cc.textMuted,
                    modifier = Modifier.padding(horizontal = 14.dp, vertical = 5.dp)
                )
            }
        }
    }
}

@Composable
private fun SettingsTabContent(
    selectedTab: SettingsTab,
    isExtraTabSelected: Boolean,
    extraTabContent: (@Composable () -> Unit)?,
    state: SettingsState,
    onIntent: (SettingsIntent) -> Unit,
    onShowFeedback: () -> Unit
) {
    if (isExtraTabSelected && extraTabContent != null) {
        extraTabContent()
    } else {
        when (selectedTab) {
            SettingsTab.Appearance -> AppearanceTab(
                themeMode = state.themeMode,
                onModeChange = { onIntent(SettingsIntent.ThemeModeChanged(it)) },
                onShowFeedback = onShowFeedback,
                onNavigateToAbout = { onIntent(SettingsIntent.SelectTab(SettingsTab.About)) }
            )
            SettingsTab.Security -> SecurityTab(state, onIntent)
            SettingsTab.AiAgents -> AiAgentsTab(state, onIntent)
            SettingsTab.MasterInstructions -> MasterInstructionsTab(state.masterInstructions, { onIntent(SettingsIntent.MasterInstructionsChanged(it)) }, { onIntent(SettingsIntent.Save) })
            SettingsTab.DebatePolicy -> DebatePolicyTab(state.debatePolicy, { onIntent(SettingsIntent.DebatePolicyChanged(it)) }, { onIntent(SettingsIntent.Save) })
            SettingsTab.Compaction -> CompactionTab(state.compactionSettings, { onIntent(SettingsIntent.CompactionSettingsChanged(it)) }, { onIntent(SettingsIntent.Save) })
            SettingsTab.Personas -> PersonasTab(state, onIntent)
            SettingsTab.Limits -> LimitsTab(state.tokenBudget, { onIntent(SettingsIntent.TokenBudgetChanged(it)) }, { onIntent(SettingsIntent.Save) })
            SettingsTab.Connection -> ConnectionTab(
                connectionLabel = state.connectionLabel,
                onSwitch = { onIntent(SettingsIntent.SwitchConnectionRequested) },
                onNavigateToLogs = { onIntent(SettingsIntent.SelectTab(SettingsTab.Logs)) }
            )
            SettingsTab.Logs -> LogsTab()
            SettingsTab.About -> AboutTab(state.connectionLabel, state.legalConsent, onShowFeedback)
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(
    state: SettingsState,
    onIntent: (SettingsIntent) -> Unit,
    onBack: () -> Unit,
    extraTabLabel: String? = null,
    extraTabContent: (@Composable () -> Unit)? = null,
    isCompact: Boolean = false,
    modifier: Modifier = Modifier
) {
    val cc = LocalCcColors.current
    val snackbarHostState = remember { SnackbarHostState() }
    var isExtraTabSelected by remember { mutableStateOf(false) }
    var showAboutDialog by remember { mutableStateOf(false) }
    var showFeedbackDialog by remember { mutableStateOf(false) }
    val coroutineScope = rememberCoroutineScope()

    val (tabTitle, tabSubtitle) = remember(state.selectedTab, isExtraTabSelected, extraTabLabel) {
        getTabTitleAndSubtitle(state.selectedTab, isExtraTabSelected, extraTabLabel)
    }

    Box(
        modifier = modifier
            .fillMaxSize()
            .background(cc.bg)
            .onPreviewKeyEvent { event ->
                if (event.type == KeyEventType.KeyDown && event.key == Key.Escape) {
                    onBack()
                    true
                } else false
            }
    ) {
        BoxWithConstraints(modifier = Modifier.fillMaxSize()) {
            val isSmallWidth = maxWidth < 760.dp || isCompact
            if (isSmallWidth) {
                // ── Responsive Mobile Layout ──
                Scaffold(
                    topBar = {
                        TopAppBar(
                            title = { Text(tabTitle) },
                            navigationIcon = {
                                IconButton(onClick = onBack) {
                                    Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                                }
                            },
                            colors = TopAppBarDefaults.topAppBarColors(containerColor = cc.bg, titleContentColor = cc.textPrimary)
                        )
                    },
                    snackbarHost = { com.dialex.ui.ThemedSnackbarHost(snackbarHostState) },
                    containerColor = cc.bg
                ) { padding ->
                    Column(
                        Modifier
                            .fillMaxSize()
                            .padding(padding)
                    ) {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .background(cc.panelAlt)
                                .horizontalScroll(rememberScrollState())
                                .padding(horizontal = 12.dp, vertical = 8.dp),
                            horizontalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            SettingsTab.entries.forEach { tab ->
                                val isSelected = state.selectedTab == tab && !isExtraTabSelected
                                val bg = if (isSelected) (if (cc.isDark) Color(0xFF262632) else Color(0xFFE5E7EB)) else Color.Transparent
                                val color = if (isSelected) cc.textPrimary else cc.textMuted
                                Surface(
                                    shape = RoundedCornerShape(8.dp),
                                    color = bg,
                                    modifier = Modifier
                                        .clip(RoundedCornerShape(8.dp))
                                        .clickable {
                                            isExtraTabSelected = false
                                            onIntent(SettingsIntent.SelectTab(tab))
                                        }
                                        .padding(horizontal = 12.dp, vertical = 6.dp)
                                ) {
                                    Text(
                                        tab.label,
                                        color = color,
                                        style = MaterialTheme.typography.bodyMedium.copy(
                                            fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal,
                                            fontSize = 13.sp
                                        )
                                    )
                                }
                            }
                        }
                        Box(Modifier.weight(1f).fillMaxWidth().padding(16.dp)) {
                            SettingsTabContent(
                                selectedTab = state.selectedTab,
                                isExtraTabSelected = isExtraTabSelected,
                                extraTabContent = extraTabContent,
                                state = state,
                                onIntent = onIntent,
                                onShowFeedback = { showFeedbackDialog = true }
                            )
                        }
                    }
                }
            } else {
                // ── Two-Pane Layout matching Reference Design ──
                Row(Modifier.fillMaxSize()) {
                    // Left Pane: Navigation Rail (250dp) with reasonable top padding from traffic lights
                    Column(
                        modifier = Modifier
                            .width(250.dp)
                            .fillMaxHeight()
                            .background(cc.panelAlt.copy(alpha = 0.5f))
                            .padding(start = 14.dp, end = 14.dp, top = 44.dp, bottom = 18.dp)
                    ) {
                        // Title with Back Button (clears traffic lights comfortably)
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(start = 2.dp, bottom = 14.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            ThemedTooltipBox("Back (Esc)") {
                                IconButton(
                                    onClick = onBack,
                                    modifier = Modifier.size(32.dp)
                                ) {
                                    Icon(
                                        Icons.AutoMirrored.Filled.ArrowBack,
                                        contentDescription = "Back",
                                        tint = cc.textPrimary,
                                        modifier = Modifier.size(18.dp)
                                    )
                                }
                            }
                            Text(
                                "Settings",
                                style = MaterialTheme.typography.titleMedium.copy(
                                    fontWeight = FontWeight.SemiBold,
                                    fontSize = 16.sp
                                ),
                                color = cc.textPrimary
                            )
                        }

                        SettingsNavPill(
                            label = "General",
                            isSelected = state.selectedTab == SettingsTab.Appearance && !isExtraTabSelected
                        ) {
                            isExtraTabSelected = false
                            onIntent(SettingsIntent.SelectTab(SettingsTab.Appearance))
                        }
                        SettingsNavPill(
                            label = "Security & Profiles",
                            isSelected = state.selectedTab == SettingsTab.Security && !isExtraTabSelected
                        ) {
                            isExtraTabSelected = false
                            onIntent(SettingsIntent.SelectTab(SettingsTab.Security))
                        }
                        SettingsNavPill(
                            label = "AI Agents & Models",
                            isSelected = state.selectedTab == SettingsTab.AiAgents && !isExtraTabSelected
                        ) {
                            isExtraTabSelected = false
                            onIntent(SettingsIntent.SelectTab(SettingsTab.AiAgents))
                        }
                        SettingsNavPill(
                            label = "Personas",
                            isSelected = state.selectedTab == SettingsTab.Personas && !isExtraTabSelected
                        ) {
                            isExtraTabSelected = false
                            onIntent(SettingsIntent.SelectTab(SettingsTab.Personas))
                        }
                        SettingsNavPill(
                            label = "Master Instructions",
                            isSelected = state.selectedTab == SettingsTab.MasterInstructions && !isExtraTabSelected
                        ) {
                            isExtraTabSelected = false
                            onIntent(SettingsIntent.SelectTab(SettingsTab.MasterInstructions))
                        }
                        SettingsNavPill(
                            label = "Autopilot & Defaults",
                            isSelected = state.selectedTab == SettingsTab.DebatePolicy && !isExtraTabSelected
                        ) {
                            isExtraTabSelected = false
                            onIntent(SettingsIntent.SelectTab(SettingsTab.DebatePolicy))
                        }

                        Spacer(Modifier.height(18.dp))

                        // Section: App & Engine
                        Text(
                            "APP & ENGINE",
                            style = MaterialTheme.typography.labelSmall.copy(
                                fontSize = 11.sp,
                                fontWeight = FontWeight.SemiBold,
                                letterSpacing = 0.8.sp
                            ),
                            color = cc.textMuted.copy(alpha = 0.7f),
                            modifier = Modifier.padding(start = 8.dp, bottom = 6.dp)
                        )
                        SettingsNavPill(
                            label = "Compaction",
                            isSelected = state.selectedTab == SettingsTab.Compaction && !isExtraTabSelected
                        ) {
                            isExtraTabSelected = false
                            onIntent(SettingsIntent.SelectTab(SettingsTab.Compaction))
                        }
                        SettingsNavPill(
                            label = "Limits & Budget",
                            isSelected = state.selectedTab == SettingsTab.Limits && !isExtraTabSelected
                        ) {
                            isExtraTabSelected = false
                            onIntent(SettingsIntent.SelectTab(SettingsTab.Limits))
                        }
                        SettingsNavPill(
                            label = "Connection",
                            isSelected = state.selectedTab == SettingsTab.Connection && !isExtraTabSelected
                        ) {
                            isExtraTabSelected = false
                            onIntent(SettingsIntent.SelectTab(SettingsTab.Connection))
                        }
                        SettingsNavPill(
                            label = "About & Legal",
                            isSelected = state.selectedTab == SettingsTab.About && !isExtraTabSelected
                        ) {
                            isExtraTabSelected = false
                            onIntent(SettingsIntent.SelectTab(SettingsTab.About))
                        }
                        if (extraTabLabel != null && extraTabContent != null) {
                            SettingsNavPill(
                                label = extraTabLabel,
                                isSelected = isExtraTabSelected
                            ) {
                                isExtraTabSelected = true
                            }
                        }

                        Spacer(Modifier.weight(1f))

                        // Bottom Footer
                        ThemedTooltipBox("About Dialex & Legal") {
                            Text(
                                "Dialex v1.0.0",
                                style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.sp),
                                color = cc.textMuted.copy(alpha = 0.6f),
                                modifier = Modifier
                                    .padding(start = 8.dp)
                                    .clickable {
                                        isExtraTabSelected = false
                                        onIntent(SettingsIntent.SelectTab(SettingsTab.About))
                                    }
                            )
                        }
                    }

                    VerticalDivider(color = cc.border.copy(alpha = 0.35f), thickness = 0.75.dp)

                    // Right Pane: Top Title + Subtitle + Close 'X' Button, and Content
                    Column(
                        modifier = Modifier
                            .weight(1f)
                            .fillMaxHeight()
                    ) {
                        // Header Row
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(start = 36.dp, end = 36.dp, top = 26.dp, bottom = 14.dp),
                            horizontalArrangement = Arrangement.SpaceBetween,
                            verticalAlignment = Alignment.Top
                        ) {
                            Column(modifier = Modifier.weight(1f).padding(end = 16.dp)) {
                                Text(
                                    tabTitle,
                                    style = MaterialTheme.typography.titleMedium.copy(
                                        fontWeight = FontWeight.SemiBold,
                                        fontSize = 16.5.sp
                                    ),
                                    color = cc.textPrimary
                                )
                                Spacer(Modifier.height(3.dp))
                                Text(
                                    tabSubtitle,
                                    style = MaterialTheme.typography.bodyMedium.copy(fontSize = 12.sp),
                                    color = cc.textMuted
                                )
                            }
                            ThemedTooltipBox("Close (Esc)") {
                                IconButton(
                                    onClick = onBack,
                                    modifier = Modifier.size(32.dp)
                                ) {
                                    Icon(
                                        Icons.Default.Close,
                                        contentDescription = "Close",
                                        tint = cc.textMuted,
                                        modifier = Modifier.size(18.dp)
                                    )
                                }
                            }
                        }

                        HorizontalDivider(color = cc.border.copy(alpha = 0.2f), thickness = 0.75.dp)

                        // Content Canvas
                        Box(
                            modifier = Modifier
                                .weight(1f)
                                .fillMaxWidth()
                                .padding(horizontal = 40.dp, vertical = 24.dp)
                        ) {
                            SettingsTabContent(
                                selectedTab = state.selectedTab,
                                isExtraTabSelected = isExtraTabSelected,
                                extraTabContent = extraTabContent,
                                state = state,
                                onIntent = onIntent,
                                onShowFeedback = { showFeedbackDialog = true }
                            )
                        }
                    }
                }
            }
        }

        com.dialex.ui.ThemedSnackbarHost(
            hostState = snackbarHostState,
            modifier = Modifier.align(Alignment.BottomCenter).padding(bottom = 16.dp)
        )
    }

    if (showAboutDialog) {
        AboutDialog(
            authorName = "Arun Electra",
            onDismiss = { showAboutDialog = false },
            onOpenFullAbout = {
                isExtraTabSelected = false
                onIntent(SettingsIntent.SelectTab(SettingsTab.About))
            }
        )
    }

    if (showFeedbackDialog) {
        val chatDisplayState = LocalChatDisplaySettings.current
        FeedbackDialog(
            initialBotToken = chatDisplayState.value.telegramBotToken,
            initialChatId = chatDisplayState.value.telegramChatId,
            onDismiss = { showFeedbackDialog = false },
            onSuccess = {
                showFeedbackDialog = false
                coroutineScope.launch {
                    snackbarHostState.showSnackbar("Feedback sent successfully! Thank you.")
                }
            }
        )
    }

    if (state.showSwitchConnectionConfirmation) {
        AlertDialog(
            onDismissRequest = { onIntent(SettingsIntent.SwitchConnectionCancelled) },
            title = { Text("Switch connection") },
            text = { Text("Disconnect from the current engine? You'll be asked to reconnect next.") },
            confirmButton = { TextButton(onClick = { onIntent(SettingsIntent.SwitchConnectionConfirmed) }) { Text("Switch") } },
            dismissButton = { TextButton(onClick = { onIntent(SettingsIntent.SwitchConnectionCancelled) }) { Text("Cancel") } }
        )
    }

    if (state.showLockSetupDialog) {
        ProfileLockSetupDialog(
            currentConfig = state.activeProfile?.lockConfig ?: ProfileLockConfig.None,
            onSaveLockConfig = { onIntent(SettingsIntent.UpdateProfileLock(it)) },
            onDismiss = { onIntent(SettingsIntent.DismissLockSetupDialog) }
        )
    }

    LaunchedEffect(state.error) {
        if (state.error != null) {
            snackbarHostState.showSnackbar(state.error)
            onIntent(SettingsIntent.DismissError)
        }
    }
}

@Composable
private fun AppearanceTab(
    themeMode: ThemeMode,
    onModeChange: (ThemeMode) -> Unit,
    onShowFeedback: () -> Unit,
    onNavigateToAbout: () -> Unit
) {
    val cc = LocalCcColors.current
    val chatDisplayState = LocalChatDisplaySettings.current
    val chatDisplaySettings = chatDisplayState.value

    Column(
        modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(24.dp)
    ) {
        // ── Interface Theme ──
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(
                "Interface Theme",
                style = MaterialTheme.typography.titleMedium.copy(
                    fontWeight = FontWeight.SemiBold,
                    fontSize = 14.sp
                ),
                color = cc.textPrimary
            )
            SettingCard {
                SettingRow(
                    title = "Color Theme",
                    description = "Choose between System default, Light mode, or Dark mode."
                ) {
                    SubtleSegmentedControl(
                        options = listOf(
                            ThemeMode.SYSTEM to "System",
                            ThemeMode.LIGHT to "Light",
                            ThemeMode.DARK to "Dark"
                        ),
                        selected = themeMode,
                        onSelect = onModeChange
                    )
                }
            }
        }

        // ── Chat Display & Aesthetics ──
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(
                "Chat Display & Bubbles",
                style = MaterialTheme.typography.titleMedium.copy(
                    fontWeight = FontWeight.SemiBold,
                    fontSize = 14.sp
                ),
                color = cc.textPrimary
            )
            SettingCard {
                SettingRow(
                    title = "Distinct Agent Bubble Backgrounds",
                    description = "Tint each agent's message bubble with their distinct AI provider color."
                ) {
                    AestheticSwitch(
                        checked = chatDisplaySettings.separateAgentBubbleBackgrounds,
                        onCheckedChange = {
                            chatDisplayState.value = chatDisplaySettings.copy(separateAgentBubbleBackgrounds = it)
                        }
                    )
                }

                HorizontalDivider(color = cc.border.copy(alpha = 0.35f), thickness = 0.5.dp)

                SettingRow(
                    title = "Artifact Generation Strips",
                    description = "Show deliverable and artifact generation notifications directly in the chat stream."
                ) {
                    AestheticSwitch(
                        checked = chatDisplaySettings.showArtifactGenerationStrips,
                        onCheckedChange = {
                            chatDisplayState.value = chatDisplaySettings.copy(showArtifactGenerationStrips = it)
                        }
                    )
                }

                HorizontalDivider(color = cc.border.copy(alpha = 0.35f), thickness = 0.5.dp)

                SettingRow(
                    title = "Compaction Event Badges",
                    description = "Display context compaction summary pills inline within the debate transcript."
                ) {
                    AestheticSwitch(
                        checked = chatDisplaySettings.showCompactionPills,
                        onCheckedChange = {
                            chatDisplayState.value = chatDisplaySettings.copy(showCompactionPills = it)
                        }
                    )
                }
            }
        }

        // ── Bug Report & Developer Feedback ──
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(
                "Bug Report & Developer Feedback",
                style = MaterialTheme.typography.titleMedium.copy(
                    fontWeight = FontWeight.SemiBold,
                    fontSize = 14.sp
                ),
                color = cc.textPrimary
            )
            SettingCard {
                SettingRow(
                    title = "Send Feedback or Report an Issue",
                    description = "Encountered a bug or have a suggestion? Send a direct report with diagnostic details to the development team via Telegram."
                ) {
                    Button(
                        onClick = onShowFeedback,
                        shape = RoundedCornerShape(8.dp),
                        colors = ButtonDefaults.buttonColors(
                            containerColor = cc.accent,
                            contentColor = Color.White
                        ),
                        contentPadding = PaddingValues(horizontal = 14.dp, vertical = 7.dp)
                    ) {
                        Icon(
                            Icons.Outlined.BugReport,
                            contentDescription = null,
                            modifier = Modifier.size(16.dp),
                            tint = Color.White
                        )
                        Spacer(Modifier.width(6.dp))
                        Text(
                            "Send feedback",
                            style = MaterialTheme.typography.bodySmall.copy(
                                fontSize = 12.5.sp,
                                fontWeight = FontWeight.Medium
                            )
                        )
                    }
                }
            }
        }

        // ── About Dialex ──
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(
                "About Dialex",
                style = MaterialTheme.typography.titleMedium.copy(
                    fontWeight = FontWeight.SemiBold,
                    fontSize = 14.sp
                ),
                color = cc.textPrimary
            )
            SettingCard {
                SelectionContainer {
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(16.dp),
                        verticalArrangement = Arrangement.spacedBy(14.dp)
                    ) {
                        // Header with Branding & Version
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(14.dp)
                        ) {
                            DialexLogoView(
                                size = 44.dp,
                                modifier = Modifier.clip(RoundedCornerShape(10.dp))
                            )

                            Column(modifier = Modifier.weight(1f)) {
                                Row(
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                                ) {
                                    Text(
                                        "Dialex",
                                        style = MaterialTheme.typography.titleMedium.copy(
                                            fontWeight = FontWeight.SemiBold,
                                            fontSize = 15.sp
                                        ),
                                        color = cc.textPrimary
                                    )
                                    Surface(
                                        shape = RoundedCornerShape(4.dp),
                                        color = cc.panelAlt,
                                        border = BorderStroke(0.75.dp, cc.border.copy(alpha = 0.6f))
                                    ) {
                                        Text(
                                            "v1.0.0",
                                            style = MaterialTheme.typography.labelSmall.copy(
                                                fontSize = 10.sp,
                                                fontWeight = FontWeight.Medium
                                            ),
                                            color = cc.textMuted,
                                            modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp)
                                        )
                                    }
                                }
                                Spacer(Modifier.height(2.dp))
                                Text(
                                    "Autonomous Multi-Agent AI Debate & Deliberation Platform",
                                    style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                                    color = cc.textMuted
                                )
                            }
                        }

                        HorizontalDivider(color = cc.border.copy(alpha = 0.25f), thickness = 0.75.dp)

                        // Product & Publisher Information
                        Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.SpaceBetween,
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Text("Publisher", style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.5.sp), color = cc.textMuted)
                                Text("Kolta Labs", style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.5.sp, fontWeight = FontWeight.Medium), color = cc.textPrimary)
                            }
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.SpaceBetween,
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Text("Engine Runtime", style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.5.sp), color = cc.textMuted)
                                Text("Dialex Core Engine Daemon", style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.5.sp, fontWeight = FontWeight.Medium), color = cc.textPrimary)
                            }
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.SpaceBetween,
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Text("Execution Modes", style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.5.sp), color = cc.textMuted)
                                Text("Local Subprocess CLI & Remote Cloud Host", style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.5.sp, fontWeight = FontWeight.Medium), color = cc.textPrimary)
                            }
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.SpaceBetween,
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Text("License", style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.5.sp), color = cc.textMuted)
                                Text("PolyForm Noncommercial 1.0.0", style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.5.sp, fontWeight = FontWeight.Medium), color = cc.textPrimary)
                            }
                        }

                        HorizontalDivider(color = cc.border.copy(alpha = 0.25f), thickness = 0.75.dp)

                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(8.dp))
                                .clickable { onNavigateToAbout() }
                                .padding(vertical = 4.dp),
                            horizontalArrangement = Arrangement.SpaceBetween,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Text(
                                "View Complete Architecture, License & Legal Details",
                                style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp, color = cc.accent, fontWeight = FontWeight.Medium)
                            )
                            Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, contentDescription = null, tint = cc.accent, modifier = Modifier.size(16.dp))
                        }

                        HorizontalDivider(color = cc.border.copy(alpha = 0.25f), thickness = 0.75.dp)

                        Text(
                            "Copyright © 2026 Kolta Labs. All rights reserved.",
                            style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp),
                            color = cc.textMuted.copy(alpha = 0.7f)
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun SecurityTab(state: SettingsState, onIntent: (SettingsIntent) -> Unit) {
    val cc = LocalCcColors.current
    val activeProfile = state.activeProfile

    Column(
        modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(24.dp)
    ) {
        // Active Profile Card
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(
                "Active Connection Profile",
                style = MaterialTheme.typography.titleMedium.copy(
                    fontWeight = FontWeight.SemiBold,
                    fontSize = 14.sp
                ),
                color = cc.textPrimary
            )
            SettingCard {
                SettingRow(
                    title = activeProfile?.name ?: "Local Engine",
                    description = when (activeProfile?.type) {
                        ProfileType.REMOTE -> "Remote Engine (${activeProfile.remoteUrl ?: "No URL"})"
                        else -> "Local On-Device Engine Daemon"
                    }
                ) {
                    Surface(
                        shape = RoundedCornerShape(6.dp),
                        color = cc.panelAlt,
                        border = BorderStroke(1.dp, cc.border)
                    ) {
                        Text(
                            text = activeProfile?.type?.name ?: "LOCAL",
                            style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.Medium),
                            color = cc.textMuted,
                            modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp)
                        )
                    }
                }
            }
        }

        // Profile Access Protection Card
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(
                "Profile Security",
                style = MaterialTheme.typography.titleMedium.copy(
                    fontWeight = FontWeight.SemiBold,
                    fontSize = 14.sp
                ),
                color = cc.textPrimary
            )
            SettingCard {
                val isProtected = activeProfile?.lockConfig != null && activeProfile.lockConfig !is ProfileLockConfig.None
                val statusText = when (activeProfile?.lockConfig) {
                    is ProfileLockConfig.CustomPin -> "Protected with Custom PIN"
                    is ProfileLockConfig.DeviceBiometricOrPin -> "Protected with Device Biometrics / System PIN"
                    else -> "No lock configured"
                }

                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 14.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Column(
                        modifier = Modifier.weight(1f).padding(end = 16.dp),
                        verticalArrangement = Arrangement.spacedBy(4.dp)
                    ) {
                        Text(
                            "Profile Access Protection",
                            style = MaterialTheme.typography.bodyMedium.copy(
                                fontWeight = FontWeight.Medium,
                                fontSize = 13.5.sp
                            ),
                            color = cc.textPrimary
                        )
                        Text(
                            "Require authentication before accessing this profile. Protects your debate history, project transcripts, and stored AI API keys from unauthorized access.",
                            style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                            color = cc.textMuted
                        )
                        Spacer(Modifier.height(2.dp))
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(5.dp)
                        ) {
                            Icon(
                                if (isProtected) Icons.Filled.Lock else Icons.Filled.LockOpen,
                                contentDescription = null,
                                modifier = Modifier.size(13.dp),
                                tint = cc.textMuted
                            )
                            Text(
                                statusText,
                                style = MaterialTheme.typography.labelSmall.copy(
                                    fontSize = 11.sp,
                                    fontWeight = if (isProtected) FontWeight.Medium else FontWeight.Normal
                                ),
                                color = cc.textPrimary
                            )
                        }
                    }

                    OutlinedButton(
                        onClick = { onIntent(SettingsIntent.ShowLockSetupDialog) },
                        shape = RoundedCornerShape(6.dp),
                        contentPadding = PaddingValues(horizontal = 12.dp, vertical = 4.dp),
                        modifier = Modifier.height(28.dp)
                    ) {
                        Text(
                            if (isProtected) "Change" else "Set Up",
                            style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.5.sp),
                            color = cc.textPrimary
                        )
                    }
                }
            }
        }

        // Available Profiles Switcher Card
        if (state.availableProfiles.size > 1) {
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(
                    "Switch Connection Profile",
                    style = MaterialTheme.typography.titleMedium.copy(
                        fontWeight = FontWeight.SemiBold,
                        fontSize = 14.sp
                    ),
                    color = cc.textPrimary
                )
                SettingCard {
                    state.availableProfiles.forEach { profile ->
                        val isSelected = profile.id == activeProfile?.id
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(12.dp),
                            horizontalArrangement = Arrangement.SpaceBetween,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Column {
                                Text(
                                    profile.name,
                                    style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.Medium),
                                    color = cc.textPrimary
                                )
                                Text(
                                    if (profile.type == ProfileType.LOCAL) "Local Engine" else "Remote: ${profile.remoteUrl ?: ""}",
                                    style = MaterialTheme.typography.bodySmall,
                                    color = cc.textMuted
                                )
                            }
                            if (!isSelected) {
                                OutlinedButton(
                                    onClick = { onIntent(SettingsIntent.SwitchProfile(profile.id)) },
                                    modifier = Modifier.height(32.dp)
                                ) {
                                    Text("Switch", style = MaterialTheme.typography.labelSmall)
                                }
                            } else {
                                Surface(
                                    shape = RoundedCornerShape(4.dp),
                                    color = Color(0xFF4CAF50).copy(alpha = 0.12f),
                                    border = BorderStroke(0.75.dp, Color(0xFF4CAF50).copy(alpha = 0.35f))
                                ) {
                                    Text(
                                        "Active",
                                        style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.5.sp),
                                        color = Color(0xFF4CAF50),
                                        modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp)
                                    )
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun AiAgentsTab(state: SettingsState, onIntent: (SettingsIntent) -> Unit) {
    val scrollState = rememberScrollState()
    val cc = LocalCcColors.current
    var editingProvider by remember { mutableStateOf<com.dialex.model.Provider?>(null) }
    var cliManagementDialogOpen by remember { mutableStateOf(false) }

    val providers = listOf(
        com.dialex.model.Provider.ANTHROPIC,
        com.dialex.model.Provider.OPENAI,
        com.dialex.model.Provider.GEMINI,
        com.dialex.model.Provider.GROK,
        com.dialex.model.Provider.DEEPSEEK,
        com.dialex.model.Provider.MISTRAL,
        com.dialex.model.Provider.OLLAMA,
        com.dialex.model.Provider.CUSTOM,
    )

    val isCheckingCli = state.cliStatusAsync is AsyncState.Loading

    Column(Modifier.fillMaxSize()) {
        Row(
            Modifier.fillMaxWidth().padding(bottom = 12.dp),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            ThemedTooltipBox("Manage, install & authenticate CLI agent runners") {
                GradientButton(
                    text = "Manage CLI Tools",
                    icon = Icons.Outlined.Terminal,
                    onClick = { cliManagementDialogOpen = true },
                    height = 32.dp,
                    contentPadding = PaddingValues(horizontal = 14.dp, vertical = 6.dp)
                )
            }

            ThemedTooltipBox("Re-check system CLI binaries") {
                OutlinedButton(
                    onClick = { onIntent(SettingsIntent.RecheckCliStatus) },
                    enabled = !isCheckingCli,
                    shape = RoundedCornerShape(8.dp),
                    contentPadding = PaddingValues(horizontal = 12.dp, vertical = 6.dp),
                    modifier = Modifier.height(32.dp)
                ) {
                    if (isCheckingCli) {
                        CircularProgressIndicator(
                            modifier = Modifier.size(12.dp),
                            strokeWidth = 1.5.dp,
                            color = cc.textPrimary
                        )
                        Spacer(Modifier.width(6.dp))
                    } else {
                        Icon(Icons.Filled.Refresh, "Refresh CLI Status", tint = cc.textMuted, modifier = Modifier.size(14.dp))
                        Spacer(Modifier.width(6.dp))
                        Text("Re-check CLI Status", style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.5.sp))
                    }
                }
            }
        }

        Column(
            Modifier.weight(1f).verticalScroll(scrollState),
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            // Dedicated Top Section: Ollama & Local LLM Manager
            OllamaLocalLlmSection(state = state, onIntent = onIntent, cc = cc)

            Text(
                "Cloud & Standard Model Providers",
                style = MaterialTheme.typography.bodySmall.copy(fontWeight = FontWeight.SemiBold, fontSize = 12.sp),
                color = cc.textMuted,
                modifier = Modifier.padding(start = 2.dp, top = 4.dp, bottom = 2.dp)
            )

            providers.forEach { provider ->
                val status = state.providerStatuses.firstOrNull { it.provider == provider }
                val hasKey = status?.apiKeyConfigured == true || state.apiKeys.forProvider(provider).isNotBlank()
                val cliCmd = state.cliCommands.forProvider(provider)
                val params = state.agentDefaults.forProvider(provider)

                AgentProviderSectionCard(
                    provider = provider,
                    status = status,
                    hasKey = hasKey,
                    cliCommand = cliCmd,
                    agentParams = params,
                    cc = cc,
                    onEdit = { editingProvider = provider }
                )
            }
            Spacer(Modifier.height(16.dp))
        }
    }

    if (cliManagementDialogOpen) {
        CliManagementDialog(
            providerStatuses = state.providerStatuses,
            onDismiss = { cliManagementDialogOpen = false },
            onRecheck = { onIntent(SettingsIntent.RecheckCliStatus) }
        )
    }

    // Modal Edit Dialog for a specific Provider
    editingProvider?.let { provider ->
        val status = state.providerStatuses.firstOrNull { it.provider == provider }
        val hasKey = status?.apiKeyConfigured == true || state.apiKeys.forProvider(provider).isNotBlank()
        val currentParams = state.agentDefaults.forProvider(provider)

        EditAgentProviderDialog(
            provider = provider,
            hasApiKey = hasKey,
            initialCliCommand = state.cliCommands.forProvider(provider),
            initialParams = currentParams,
            onDismiss = { editingProvider = null },
            onSave = { newKey, newCmd, newParams ->
                if (newKey != null && newKey.isNotBlank()) {
                    onIntent(SettingsIntent.SetApiKey(provider, newKey))
                }
                val updatedCmds = when (provider) {
                    com.dialex.model.Provider.ANTHROPIC -> state.cliCommands.copy(anthropic = newCmd)
                    com.dialex.model.Provider.OPENAI -> state.cliCommands.copy(openai = newCmd)
                    com.dialex.model.Provider.GEMINI -> state.cliCommands.copy(gemini = newCmd)
                    com.dialex.model.Provider.GROK -> state.cliCommands.copy(grok = newCmd)
                    com.dialex.model.Provider.DEEPSEEK -> state.cliCommands.copy(deepseek = newCmd)
                    com.dialex.model.Provider.MISTRAL -> state.cliCommands.copy(mistral = newCmd)
                    com.dialex.model.Provider.OLLAMA -> state.cliCommands.copy(ollama = newCmd)
                    com.dialex.model.Provider.CUSTOM -> state.cliCommands.copy(custom = newCmd)
                }
                val updatedDefaults = state.agentDefaults.updateForProvider(provider, newParams)
                onIntent(SettingsIntent.CliCommandsChanged(updatedCmds))
                onIntent(SettingsIntent.AgentDefaultsChanged(updatedDefaults))
                onIntent(SettingsIntent.Save)
                editingProvider = null
            },
            onRemoveKey = {
                onIntent(SettingsIntent.RemoveApiKey(provider))
                editingProvider = null
            }
        )
    }
}

@Composable
private fun AgentProviderSectionCard(
    provider: com.dialex.model.Provider,
    status: ProviderStatus?,
    hasKey: Boolean,
    cliCommand: String,
    agentParams: com.dialex.model.AgentParams,
    cc: com.dialex.theme.CcPalette,
    onEdit: () -> Unit
) {
    val isCustom = provider == com.dialex.model.Provider.CUSTOM
    val providerSupportsCli = com.dialex.service.hasDedicatedCli(provider) ||
        (isCustom && cliCommand.isNotBlank())
    val hasCli = if (isCustom) cliCommand.isNotBlank() else (status?.cliFound == true)
    val cliLoggedIn = status?.cliLoggedIn == true

    val isLive = hasKey || (providerSupportsCli && hasCli && cliLoggedIn)
    val isNotLoggedIn = providerSupportsCli && hasCli && !cliLoggedIn
    val dotColor = when {
        isLive -> Color(0xFF4CAF50)
        isNotLoggedIn -> cc.agentThird
        else -> cc.textMuted.copy(alpha = 0.5f)
    }

    SettingCard {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 14.dp, vertical = 11.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            Row(
                modifier = Modifier.weight(1f).padding(end = 12.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Box(
                    Modifier.size(7.dp)
                        .clip(CircleShape)
                        .background(dotColor)
                )
                Text(
                    provider.brandName(),
                    style = MaterialTheme.typography.bodyMedium.copy(
                        fontWeight = FontWeight.Medium,
                        fontSize = 13.5.sp
                    ),
                    color = cc.textPrimary
                )

                // API Key Badge
                if (!isCustom) {
                    Surface(
                        shape = RoundedCornerShape(4.dp),
                        color = if (hasKey) Color(0xFF4CAF50).copy(alpha = 0.12f) else cc.panelAlt,
                        border = BorderStroke(0.75.dp, if (hasKey) Color(0xFF4CAF50).copy(alpha = 0.35f) else cc.border.copy(alpha = 0.3f))
                    ) {
                        Text(
                            if (hasKey) "API Key Active" else "No API Key",
                            style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.5.sp),
                            color = if (hasKey) Color(0xFF4CAF50) else cc.textMuted,
                            modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp)
                        )
                    }
                }

                // CLI Badge - only show for providers that actually have a CLI!
                if (providerSupportsCli && !isCustom) {
                    val badgeText = when {
                        !hasCli -> "No CLI"
                        !cliLoggedIn -> "CLI - Not Logged In"
                        else -> "CLI Ready"
                    }
                    val badgeColor = when {
                        !hasCli -> cc.textMuted
                        !cliLoggedIn -> Color(0xFFFF9800)
                        else -> Color(0xFF4CAF50)
                    }
                    val badgeBg = when {
                        !hasCli -> cc.panelAlt
                        !cliLoggedIn -> Color(0xFFFF9800).copy(alpha = 0.12f)
                        else -> Color(0xFF4CAF50).copy(alpha = 0.12f)
                    }
                    val badgeBorder = when {
                        !hasCli -> cc.border.copy(alpha = 0.3f)
                        !cliLoggedIn -> Color(0xFFFF9800).copy(alpha = 0.35f)
                        else -> Color(0xFF4CAF50).copy(alpha = 0.35f)
                    }

                    Surface(
                        shape = RoundedCornerShape(4.dp),
                        color = badgeBg,
                        border = BorderStroke(0.75.dp, badgeBorder)
                    ) {
                        Text(
                            badgeText,
                            style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.5.sp),
                            color = badgeColor,
                            modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp)
                        )
                    }
                }

                // Sampling Params Badge
                Surface(
                    shape = RoundedCornerShape(4.dp),
                    color = cc.panelAlt,
                    border = BorderStroke(0.75.dp, cc.border.copy(alpha = 0.35f))
                ) {
                    val tempStr = "Temp: " + ((agentParams.temperature ?: 0.7) * 100).toInt() / 100.0
                    val tokensStr = "${agentParams.maxTokens ?: 4096} tok"
                    Text(
                        "$tempStr · $tokensStr",
                        style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.5.sp, fontFamily = FontFamily.Monospace),
                        color = cc.textMuted,
                        modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp)
                    )
                }

                if (isCustom && cliCommand.isNotBlank()) {
                    Text(
                        cliCommand,
                        style = MaterialTheme.typography.bodySmall.copy(fontFamily = androidx.compose.ui.text.font.FontFamily.Monospace, fontSize = 11.sp),
                        color = cc.textMuted,
                        maxLines = 1
                    )
                }
            }

            ThemedTooltipBox("Configure ${provider.brandName()}") {
                OutlinedButton(
                    onClick = onEdit,
                    shape = RoundedCornerShape(6.dp),
                    contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp),
                    modifier = Modifier.height(28.dp)
                ) {
                    Icon(Icons.Default.Edit, contentDescription = null, modifier = Modifier.size(13.dp), tint = cc.textPrimary)
                    Spacer(Modifier.width(4.dp))
                    Text("Edit", style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.5.sp), color = cc.textPrimary)
                }
            }
        }
    }
}

@Composable
private fun OllamaLocalLlmSection(
    state: SettingsState,
    onIntent: (SettingsIntent) -> Unit,
    cc: com.dialex.theme.CcPalette
) {
    var endpointInput by remember(state.apiKeys.ollama) { mutableStateOf(state.apiKeys.ollama.ifBlank { "http://localhost:11434" }) }
    var customPullModelName by remember { mutableStateOf("") }

    Surface(
        shape = RoundedCornerShape(12.dp),
        color = cc.panel,
        border = BorderStroke(1.dp, cc.accent.copy(alpha = 0.35f)),
        modifier = Modifier.fillMaxWidth().padding(bottom = 12.dp)
    ) {
        Column(
            modifier = Modifier.fillMaxWidth().padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            // Header Row: Title + Health Badge + Refresh Button
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    Surface(
                        shape = CircleShape,
                        color = Color(0xFF0EA5E9).copy(alpha = 0.15f),
                        modifier = Modifier.size(36.dp)
                    ) {
                        Box(contentAlignment = Alignment.Center) {
                            Icon(
                                Icons.Outlined.SmartToy,
                                contentDescription = null,
                                tint = Color(0xFF0EA5E9),
                                modifier = Modifier.size(20.dp)
                            )
                        }
                    }

                    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            Text(
                                "Ollama & Local LLMs",
                                style = MaterialTheme.typography.titleSmall.copy(fontWeight = FontWeight.Bold, fontSize = 14.sp),
                                color = cc.textPrimary
                            )
                            when (val h = state.ollamaHealth) {
                                is com.dialex.service.OllamaHealthStatus.Online -> {
                                    Surface(
                                        shape = RoundedCornerShape(4.dp),
                                        color = Color(0xFF10B981).copy(alpha = 0.15f),
                                        border = BorderStroke(0.5.dp, Color(0xFF10B981).copy(alpha = 0.4f))
                                    ) {
                                        Text(
                                            "ONLINE · v${h.version} · ${h.modelCount} models",
                                            style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.sp, fontWeight = FontWeight.Bold),
                                            color = Color(0xFF10B981),
                                            modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp)
                                        )
                                    }
                                }
                                is com.dialex.service.OllamaHealthStatus.Offline -> {
                                    Surface(
                                        shape = RoundedCornerShape(4.dp),
                                        color = MaterialTheme.colorScheme.error.copy(alpha = 0.12f),
                                        border = BorderStroke(0.5.dp, MaterialTheme.colorScheme.error.copy(alpha = 0.4f))
                                    ) {
                                        Text(
                                            "OFFLINE",
                                            style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.sp, fontWeight = FontWeight.Bold),
                                            color = MaterialTheme.colorScheme.error,
                                            modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp)
                                        )
                                    }
                                }
                                com.dialex.service.OllamaHealthStatus.Checking -> {
                                    Text(
                                        "Checking connection...",
                                        style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.5.sp),
                                        color = cc.textMuted
                                    )
                                }
                            }
                        }
                        Text(
                            "Zero-cost, 100% private on-device deliberations with locally downloaded models.",
                            style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp),
                            color = cc.textMuted
                        )
                    }
                }

                // Refresh Button
                ThemedTooltipBox("Re-check Ollama status & installed models") {
                    IconButton(
                        onClick = { onIntent(SettingsIntent.RefreshOllama) },
                        modifier = Modifier.size(30.dp)
                    ) {
                        Icon(
                            Icons.Default.Refresh,
                            contentDescription = "Refresh Ollama",
                            tint = cc.textMuted,
                            modifier = Modifier.size(16.dp)
                        )
                    }
                }
            }

            // Endpoint Row
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(6.dp))
                    .background(cc.panelAlt)
                    .padding(horizontal = 10.dp, vertical = 6.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Text(
                    "Endpoint:",
                    style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.sp, fontWeight = FontWeight.Medium),
                    color = cc.textMuted
                )
                BasicTextField(
                    value = endpointInput,
                    onValueChange = {
                        endpointInput = it
                        onIntent(SettingsIntent.UpdateOllamaEndpoint(it))
                    },
                    singleLine = true,
                    textStyle = MaterialTheme.typography.bodySmall.copy(
                        fontSize = 12.sp,
                        color = cc.textPrimary,
                        fontFamily = androidx.compose.ui.text.font.FontFamily.Monospace
                    ),
                    cursorBrush = SolidColor(cc.accent),
                    modifier = Modifier.weight(1f)
                )
            }

            // Installed Models List / Chip Cloud
            if (state.installedOllamaModels.isNotEmpty()) {
                Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    Text(
                        "Installed Local Models (${state.installedOllamaModels.size})",
                        style = MaterialTheme.typography.bodySmall.copy(fontWeight = FontWeight.SemiBold, fontSize = 11.5.sp),
                        color = cc.textPrimary
                    )
                    Row(
                        modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        state.installedOllamaModels.forEach { model ->
                            Surface(
                                shape = RoundedCornerShape(6.dp),
                                color = cc.panelAlt,
                                border = BorderStroke(0.6.dp, cc.border.copy(alpha = 0.5f))
                            ) {
                                Row(
                                    modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp),
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                                ) {
                                    Icon(
                                        Icons.Outlined.Check,
                                        contentDescription = null,
                                        tint = Color(0xFF10B981),
                                        modifier = Modifier.size(12.dp)
                                    )
                                    Text(
                                        model.name,
                                        style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.sp, fontWeight = FontWeight.Medium),
                                        color = cc.textPrimary
                                    )
                                    val sz = model.formattedSize()
                                    if (sz.isNotBlank()) {
                                        Text(
                                            sz,
                                            style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.sp),
                                            color = cc.textMuted
                                        )
                                    }
                                }
                            }
                        }
                    }
                }
            }

            // Quick One-Click Pull Recommendations & Custom Pull Input
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(
                    "One-Click Model Installer",
                    style = MaterialTheme.typography.bodySmall.copy(fontWeight = FontWeight.SemiBold, fontSize = 11.5.sp),
                    color = cc.textPrimary
                )

                if (state.isPullingOllamaModel) {
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(6.dp))
                            .background(cc.panelAlt)
                            .padding(10.dp),
                        verticalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceBetween,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Text(
                                state.pullModelStatus.ifBlank { "Downloading model layers..." },
                                style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.sp),
                                color = cc.textPrimary
                            )
                            Text(
                                "${(state.pullModelProgress * 100).toInt()}%",
                                style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.sp, fontWeight = FontWeight.Bold),
                                color = cc.accent
                            )
                        }
                        LinearProgressIndicator(
                            progress = { state.pullModelProgress },
                            modifier = Modifier.fillMaxWidth().height(4.dp).clip(RoundedCornerShape(2.dp)),
                            color = cc.accent,
                            trackColor = cc.border.copy(alpha = 0.3f)
                        )
                    }
                } else {
                    val recommendedPulls = listOf(
                        "deepseek-r1:8b" to "DeepSeek-R1",
                        "qwen2.5-coder:7b" to "Qwen 2.5 Coder",
                        "llama3.2:3b" to "Llama 3.2 (3B)",
                        "phi4:14b" to "Phi-4"
                    )

                    Row(
                        modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                        horizontalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        recommendedPulls.forEach { (modelName, _) ->
                            val isInstalled = state.installedOllamaModels.any { it.name.startsWith(modelName.substringBefore(":")) }
                            Surface(
                                shape = RoundedCornerShape(6.dp),
                                color = if (isInstalled) cc.panelAlt.copy(alpha = 0.5f) else cc.accent.copy(alpha = 0.08f),
                                border = BorderStroke(
                                    0.6.dp,
                                    if (isInstalled) cc.border.copy(alpha = 0.4f) else cc.accent.copy(alpha = 0.35f)
                                ),
                                modifier = Modifier
                                    .clip(RoundedCornerShape(6.dp))
                                    .clickable(enabled = !isInstalled) {
                                        onIntent(SettingsIntent.PullOllamaModel(modelName))
                                    }
                            ) {
                                Row(
                                    modifier = Modifier.padding(horizontal = 8.dp, vertical = 5.dp),
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                                ) {
                                    Icon(
                                        if (isInstalled) Icons.Default.Check else Icons.Outlined.FileDownload,
                                        contentDescription = null,
                                        tint = if (isInstalled) Color(0xFF10B981) else cc.accent,
                                        modifier = Modifier.size(12.dp)
                                    )
                                    Text(
                                        if (isInstalled) "$modelName (Installed)" else "Pull $modelName",
                                        style = MaterialTheme.typography.labelSmall.copy(
                                            fontSize = 11.sp,
                                            fontWeight = if (isInstalled) FontWeight.Normal else FontWeight.Medium
                                        ),
                                        color = if (isInstalled) cc.textMuted else cc.textPrimary
                                    )
                                }
                            }
                        }
                    }

                    // Custom model pull text field
                    Row(
                        modifier = Modifier.fillMaxWidth().padding(top = 2.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        BasicTextField(
                            value = customPullModelName,
                            onValueChange = { customPullModelName = it },
                            singleLine = true,
                            textStyle = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp, color = cc.textPrimary),
                            cursorBrush = SolidColor(cc.accent),
                            modifier = Modifier
                                .weight(1f)
                                .clip(RoundedCornerShape(6.dp))
                                .background(cc.panelAlt)
                                .padding(horizontal = 8.dp, vertical = 6.dp),
                            decorationBox = { innerTextField ->
                                if (customPullModelName.isEmpty()) {
                                    Text(
                                        "Or type custom model name (e.g. mistral-nemo, gemma2:9b)...",
                                        style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp),
                                        color = cc.textMuted.copy(alpha = 0.6f)
                                    )
                                }
                                innerTextField()
                            }
                        )
                        OutlinedButton(
                            onClick = {
                                if (customPullModelName.isNotBlank()) {
                                    onIntent(SettingsIntent.PullOllamaModel(customPullModelName.trim()))
                                    customPullModelName = ""
                                }
                            },
                            enabled = customPullModelName.isNotBlank(),
                            shape = RoundedCornerShape(6.dp),
                            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp),
                            modifier = Modifier.height(28.dp)
                        ) {
                            Text("Pull", style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.sp))
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun EditAgentProviderDialog(
    provider: com.dialex.model.Provider,
    hasApiKey: Boolean,
    initialCliCommand: String,
    initialParams: com.dialex.model.AgentParams,
    onDismiss: () -> Unit,
    onSave: (newApiKey: String?, cliCommand: String, newParams: com.dialex.model.AgentParams) -> Unit,
    onRemoveKey: () -> Unit
) {
    var cliCommand by remember { mutableStateOf(initialCliCommand) }
    var newApiKey by remember { mutableStateOf("") }
    var isReplacingKey by remember { mutableStateOf(!hasApiKey) }
    var temperature by remember { mutableStateOf(initialParams.temperature ?: 0.7) }
    var topP by remember { mutableStateOf(initialParams.topP ?: 0.95) }
    var maxTokens by remember { mutableStateOf((initialParams.maxTokens ?: 4096).toString()) }
    val isCustom = provider == com.dialex.model.Provider.CUSTOM
    val isOllama = provider == com.dialex.model.Provider.OLLAMA
    val cc = LocalCcColors.current

    AlertDialog(
        onDismissRequest = onDismiss,
        title = {
            Text("Configure ${provider.brandName()}")
        },
        text = {
            Column(
                Modifier
                    .fillMaxWidth()
                    .verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(14.dp)
            ) {
                Text(
                    "Configure default execution modes, credentials, and LLM sampling parameters.",
                    style = MaterialTheme.typography.bodySmall,
                    color = cc.textMuted
                )

                OutlinedTextField(
                    value = cliCommand,
                    onValueChange = { cliCommand = it },
                    label = { Text("CLI Command") },
                    placeholder = { Text("e.g. ${if (provider == com.dialex.model.Provider.ANTHROPIC) "claude --print" else if (isOllama) "ollama run" else "agy"}") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth()
                )

                if (isOllama) {
                    OutlinedTextField(
                        value = newApiKey,
                        onValueChange = { newApiKey = it },
                        label = { Text("Ollama Endpoint URL") },
                        placeholder = { Text("http://localhost:11434") },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth()
                    )
                } else if (!isCustom) {
                    if (hasApiKey && !isReplacingKey) {
                        Surface(
                            shape = RoundedCornerShape(8.dp),
                            color = Color(0xFF4CAF50).copy(alpha = 0.08f),
                            border = BorderStroke(0.75.dp, Color(0xFF4CAF50).copy(alpha = 0.35f)),
                            modifier = Modifier.fillMaxWidth()
                        ) {
                            Column(Modifier.padding(12.dp)) {
                                Row(verticalAlignment = Alignment.CenterVertically) {
                                    Icon(Icons.Default.Lock, contentDescription = null, tint = Color(0xFF4CAF50), modifier = Modifier.size(16.dp))
                                    Spacer(Modifier.width(6.dp))
                                    Text("API Key Saved & Encrypted", style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.SemiBold), color = Color(0xFF4CAF50))
                                }
                                Spacer(Modifier.height(4.dp))
                                Text(
                                    "For security, saved keys are write-only and cannot be viewed or edited directly.",
                                    style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp),
                                    color = cc.textMuted
                                )
                                Spacer(Modifier.height(10.dp))
                                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                    OutlinedButton(
                                        onClick = { isReplacingKey = true },
                                        shape = RoundedCornerShape(6.dp),
                                        modifier = Modifier.height(30.dp)
                                    ) {
                                        Text("Replace Key", style = MaterialTheme.typography.labelSmall)
                                    }
                                    TextButton(
                                        onClick = onRemoveKey,
                                        modifier = Modifier.height(30.dp)
                                    ) {
                                        Text("Remove", color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.labelSmall)
                                    }
                                }
                            }
                        }
                    } else {
                        OutlinedTextField(
                            value = newApiKey,
                            onValueChange = { newApiKey = it },
                            label = { Text(if (hasApiKey) "New API Key (Write-Only)" else "API Key (Write-Only)") },
                            placeholder = { Text("Paste secret key...") },
                            singleLine = true,
                            visualTransformation = PasswordVisualTransformation(),
                            modifier = Modifier.fillMaxWidth()
                        )
                        if (hasApiKey) {
                            TextButton(onClick = { isReplacingKey = false }) {
                                Text("Cancel replacement", style = MaterialTheme.typography.labelSmall)
                            }
                        }
                    }
                }

                // ── Default Model Sampling Parameters ──
                HorizontalDivider(color = cc.border.copy(alpha = 0.4f))

                Text(
                    "Default Sampling & Model Parameters",
                    style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.SemiBold, fontSize = 13.sp),
                    color = cc.textPrimary
                )

                // Temperature Slider
                Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text(
                            "Temperature",
                            style = MaterialTheme.typography.bodySmall.copy(fontWeight = FontWeight.Medium),
                            color = cc.textPrimary
                        )
                        val tempFormatted = ((temperature * 100).toInt() / 100.0).toString()
                        val tempNote = when {
                            temperature < 0.35 -> "Deterministic / Factual"
                            temperature < 0.85 -> "Balanced Discussion"
                            else -> "Creative / Exploratory"
                        }
                        Text(
                            "$tempFormatted ($tempNote)",
                            style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.sp, fontFamily = FontFamily.Monospace),
                            color = cc.accent
                        )
                    }
                    Slider(
                        value = temperature.toFloat(),
                        onValueChange = { temperature = (it * 20).roundToInt() / 20.0 },
                        valueRange = 0.0f..2.0f,
                        steps = 39,
                        colors = SliderDefaults.colors(
                            thumbColor = cc.accent,
                            activeTrackColor = cc.accent,
                            inactiveTrackColor = cc.border
                        )
                    )
                }

                // Top-P Slider
                Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text(
                            "Top-P (Nucleus Sampling)",
                            style = MaterialTheme.typography.bodySmall.copy(fontWeight = FontWeight.Medium),
                            color = cc.textPrimary
                        )
                        val topPFormatted = ((topP * 100).toInt() / 100.0).toString()
                        Text(
                            topPFormatted,
                            style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.sp, fontFamily = FontFamily.Monospace),
                            color = cc.accent
                        )
                    }
                    Slider(
                        value = topP.toFloat(),
                        onValueChange = { topP = (it * 20).roundToInt() / 20.0 },
                        valueRange = 0.0f..1.0f,
                        steps = 19,
                        colors = SliderDefaults.colors(
                            thumbColor = cc.accent,
                            activeTrackColor = cc.accent,
                            inactiveTrackColor = cc.border
                        )
                    )
                }

                // Max Completion Tokens
                OutlinedTextField(
                    value = maxTokens,
                    onValueChange = { input ->
                        if (input.all { it.isDigit() }) {
                            maxTokens = input
                        }
                    },
                    label = { Text("Max Output Tokens") },
                    placeholder = { Text("4096") },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = Modifier.fillMaxWidth()
                )
            }
        },
        confirmButton = {
            GradientButton(
                text = "Save Configuration",
                onClick = {
                    val parsedTokens = maxTokens.toIntOrNull() ?: 4096
                    val params = com.dialex.model.AgentParams(
                        temperature = temperature,
                        topP = topP,
                        maxTokens = parsedTokens
                    )
                    onSave(
                        if (isReplacingKey && newApiKey.isNotBlank()) newApiKey.trim() else null,
                        cliCommand.trim(),
                        params
                    )
                },
                height = 34.dp
            )
        },
        dismissButton = {
            TextButton(onClick = onDismiss) {
                Text("Cancel")
            }
        }
    )
}

@Composable
private fun MasterInstructionsTab(value: String, onChange: (String) -> Unit, onSave: () -> Unit) {
    val cc = LocalCcColors.current
    var text by remember(value) { mutableStateOf(value) }
    var saved by remember { mutableStateOf(false) }

    LaunchedEffect(saved) {
        if (saved) {
            delay(2000)
            saved = false
        }
    }

    Column(
        modifier = Modifier.fillMaxSize(),
        verticalArrangement = Arrangement.spacedBy(16.dp)
    ) {
        SettingCard(modifier = Modifier.weight(1f)) {
            Column(modifier = Modifier.fillMaxSize().padding(16.dp)) {
                Text(
                    "Global System Prompt Template",
                    style = MaterialTheme.typography.titleMedium.copy(
                        fontWeight = FontWeight.SemiBold,
                        fontSize = 14.sp
                    ),
                    color = cc.textPrimary
                )
                Spacer(Modifier.height(4.dp))
                Text(
                    "These instructions will be prepended to the system prompt of every participant agent.",
                    style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                    color = cc.textMuted
                )
                Spacer(Modifier.height(14.dp))
                OutlinedTextField(
                    value = text,
                    onValueChange = { text = it },
                    modifier = Modifier.fillMaxWidth().weight(1f),
                    placeholder = { Text("e.g. Always state your premises clearly. Maintain analytical rigor.") },
                    shape = RoundedCornerShape(8.dp)
                )
            }
        }
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
            GradientButton(
                text = if (saved) "Instructions Saved" else "Save Master Instructions",
                onClick = {
                    onChange(text)
                    onSave()
                    saved = true
                },
                height = 34.dp
            )
        }
    }
}

@Composable
private fun CompactionTab(settings: CompactionSettings, onChange: (CompactionSettings) -> Unit, onSave: () -> Unit) {
    val cc = LocalCcColors.current
    var current by remember(settings) { mutableStateOf(settings) }
    var saved by remember { mutableStateOf(false) }
    var showCustomSettings by remember { mutableStateOf(current.customCliCommand.isNotBlank() || current.fallbackStrategy != "api") }

    val providers = listOf(
        Provider.ANTHROPIC to "Anthropic (Claude)",
        Provider.OPENAI to "OpenAI",
        Provider.GEMINI to "Google Gemini",
        Provider.GROK to "xAI Grok",
        Provider.DEEPSEEK to "DeepSeek",
        Provider.MISTRAL to "Mistral AI",
        Provider.CUSTOM to "Custom / Other"
    )

    val providerModels = mapOf(
        Provider.ANTHROPIC to listOf("claude-3-5-haiku-20241022", "claude-haiku-4-5-20251001", "claude-3-7-sonnet-20250219"),
        Provider.OPENAI to listOf("gpt-4o-mini", "gpt-4.1-mini", "gpt-4o"),
        Provider.GEMINI to listOf("gemini-2.5-flash", "gemini-2.0-flash", "gemini-1.5-flash"),
        Provider.GROK to listOf("grok-2", "grok-beta"),
        Provider.DEEPSEEK to listOf("deepseek-chat", "deepseek-reasoner"),
        Provider.MISTRAL to listOf("mistral-small-latest", "mistral-large-latest"),
        Provider.CUSTOM to listOf("custom-model")
    )

    var selectedProvider by remember {
        val p = when {
            current.primaryModel.contains("claude", ignoreCase = true) -> Provider.ANTHROPIC
            current.primaryModel.contains("gpt", ignoreCase = true) -> Provider.OPENAI
            current.primaryModel.contains("gemini", ignoreCase = true) -> Provider.GEMINI
            current.primaryModel.contains("grok", ignoreCase = true) -> Provider.GROK
            current.primaryModel.contains("deepseek", ignoreCase = true) -> Provider.DEEPSEEK
            current.primaryModel.contains("mistral", ignoreCase = true) -> Provider.MISTRAL
            else -> Provider.ANTHROPIC
        }
        mutableStateOf(p)
    }

    var providerMenuExpanded by remember { mutableStateOf(false) }
    var modelMenuExpanded by remember { mutableStateOf(false) }

    LaunchedEffect(saved) {
        if (saved) {
            delay(2000)
            saved = false
        }
    }

    Column(
        modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(20.dp)
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(
                "Context Compression Strategy",
                style = MaterialTheme.typography.titleMedium.copy(
                    fontWeight = FontWeight.SemiBold,
                    fontSize = 14.sp
                ),
                color = cc.textPrimary
            )
            Text(
                "Configure automated turn compression when debates exceed context capacity. Compaction maintains an ongoing rolling summary.",
                style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                color = cc.textMuted
            )
        }

        SettingCard {
            SettingRow(
                title = "Execution Mode: Prefer CLI Runner",
                description = "Execute summarization via locally installed CLI binary instead of direct API HTTP calls."
            ) {
                AestheticSwitch(
                    checked = current.preferCli,
                    onCheckedChange = {
                        current = current.copy(preferCli = it)
                    }
                )
            }

            HorizontalDivider(color = cc.border.copy(alpha = 0.25f), thickness = 0.75.dp)

            // Compaction Provider Dropdown
            SettingRow(
                title = "Compaction Agent Provider",
                description = "AI provider backend utilized for generating turn digests."
            ) {
                Box {
                    OutlinedButton(
                        onClick = { providerMenuExpanded = true },
                        shape = RoundedCornerShape(8.dp),
                        colors = ButtonDefaults.outlinedButtonColors(
                            containerColor = cc.panelAlt,
                            contentColor = cc.textPrimary
                        ),
                        border = BorderStroke(1.dp, cc.border.copy(alpha = 0.5f)),
                        contentPadding = PaddingValues(horizontal = 12.dp, vertical = 6.dp),
                        modifier = Modifier.height(34.dp)
                    ) {
                        Text(
                            providers.firstOrNull { it.first == selectedProvider }?.second ?: selectedProvider.name,
                            style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                            color = cc.textPrimary
                        )
                        Spacer(Modifier.width(6.dp))
                        Icon(Icons.Filled.ArrowDropDown, contentDescription = null, tint = cc.textMuted, modifier = Modifier.size(16.dp))
                    }
                    DropdownMenu(
                        expanded = providerMenuExpanded,
                        onDismissRequest = { providerMenuExpanded = false },
                        modifier = Modifier
                            .background(cc.panel)
                            .border(BorderStroke(1.dp, cc.border.copy(alpha = 0.4f)), RoundedCornerShape(10.dp))
                            .clip(RoundedCornerShape(10.dp))
                    ) {
                        providers.forEach { (prov, label) ->
                            val isSelected = prov == selectedProvider
                            DropdownMenuItem(
                                text = {
                                    Text(
                                        label,
                                        style = MaterialTheme.typography.bodySmall.copy(
                                            fontSize = 12.5.sp,
                                            fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal
                                        ),
                                        color = if (isSelected) cc.accent else cc.textPrimary
                                    )
                                },
                                colors = MenuDefaults.itemColors(
                                    textColor = cc.textPrimary,
                                    leadingIconColor = cc.textMuted
                                ),
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .background(if (isSelected) cc.panelAlt else Color.Transparent),
                                onClick = {
                                    selectedProvider = prov
                                    val defaultForProv = providerModels[prov]?.firstOrNull() ?: current.primaryModel
                                    current = current.copy(primaryModel = defaultForProv)
                                    providerMenuExpanded = false
                                }
                            )
                        }
                    }
                }
            }

            HorizontalDivider(color = cc.border.copy(alpha = 0.25f), thickness = 0.75.dp)

            // Compaction Model Dropdown
            SettingRow(
                title = "Compaction Model",
                description = "Fast, lightweight model selected for compiling rolling summaries."
            ) {
                val availableModels = providerModels[selectedProvider] ?: listOf(current.primaryModel)
                Box {
                    OutlinedButton(
                        onClick = { modelMenuExpanded = true },
                        shape = RoundedCornerShape(8.dp),
                        colors = ButtonDefaults.outlinedButtonColors(
                            containerColor = cc.panelAlt,
                            contentColor = cc.textPrimary
                        ),
                        border = BorderStroke(1.dp, cc.border.copy(alpha = 0.5f)),
                        contentPadding = PaddingValues(horizontal = 12.dp, vertical = 6.dp),
                        modifier = Modifier.height(34.dp)
                    ) {
                        Text(
                            current.primaryModel,
                            style = MaterialTheme.typography.bodySmall.copy(fontFamily = FontFamily.Monospace, fontSize = 12.sp),
                            color = cc.textPrimary
                        )
                        Spacer(Modifier.width(6.dp))
                        Icon(Icons.Filled.ArrowDropDown, contentDescription = null, tint = cc.textMuted, modifier = Modifier.size(16.dp))
                    }
                    DropdownMenu(
                        expanded = modelMenuExpanded,
                        onDismissRequest = { modelMenuExpanded = false },
                        modifier = Modifier
                            .background(cc.panel)
                            .border(BorderStroke(1.dp, cc.border.copy(alpha = 0.4f)), RoundedCornerShape(10.dp))
                            .clip(RoundedCornerShape(10.dp))
                    ) {
                        availableModels.forEach { modelName ->
                            val isSelected = modelName == current.primaryModel
                            DropdownMenuItem(
                                text = {
                                    Text(
                                        modelName,
                                        style = MaterialTheme.typography.bodySmall.copy(
                                            fontFamily = FontFamily.Monospace,
                                            fontSize = 12.sp,
                                            fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal
                                        ),
                                        color = if (isSelected) cc.accent else cc.textPrimary
                                    )
                                },
                                colors = MenuDefaults.itemColors(
                                    textColor = cc.textPrimary,
                                    leadingIconColor = cc.textMuted
                                ),
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .background(if (isSelected) cc.panelAlt else Color.Transparent),
                                onClick = {
                                    current = current.copy(primaryModel = modelName)
                                    modelMenuExpanded = false
                                }
                            )
                        }
                    }
                }
            }

            HorizontalDivider(color = cc.border.copy(alpha = 0.25f), thickness = 0.75.dp)

            // Turn Threshold
            SettingRow(
                title = "Compaction Turn Threshold",
                description = "Number of dialogue turns before automated compression kicks in."
            ) {
                OutlinedTextField(
                    value = current.compactionThreshold.toString(),
                    onValueChange = {
                        val v = it.toIntOrNull() ?: 20
                        current = current.copy(compactionThreshold = v)
                    },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    textStyle = MaterialTheme.typography.bodyMedium.copy(fontSize = 13.sp),
                    modifier = Modifier.width(110.dp),
                    shape = RoundedCornerShape(8.dp),
                    trailingIcon = {
                        Text("turns", style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.sp), color = cc.textMuted, modifier = Modifier.padding(end = 6.dp))
                    }
                )
            }
        }

        // Custom Compaction Settings Card
        SettingCard {
            SettingRow(
                title = "Configure Custom Compaction Settings",
                description = "Override the default CLI binary command or configure custom fallback strategies."
            ) {
                AestheticSwitch(
                    checked = showCustomSettings,
                    onCheckedChange = { showCustomSettings = it }
                )
            }

            if (showCustomSettings) {
                HorizontalDivider(color = cc.border.copy(alpha = 0.25f), thickness = 0.75.dp)
                SettingRow(
                    title = "Custom CLI Command",
                    description = "Explicit CLI command executed when running in CLI compaction mode."
                ) {
                    OutlinedTextField(
                        value = current.customCliCommand,
                        onValueChange = { current = current.copy(customCliCommand = it) },
                        placeholder = { Text("e.g. claude --print --model claude-haiku", style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp)) },
                        singleLine = true,
                        textStyle = MaterialTheme.typography.bodySmall.copy(fontFamily = FontFamily.Monospace, fontSize = 12.sp),
                        modifier = Modifier.width(260.dp),
                        shape = RoundedCornerShape(8.dp)
                    )
                }

                HorizontalDivider(color = cc.border.copy(alpha = 0.25f), thickness = 0.75.dp)
                SettingRow(
                    title = "Fallback Strategy",
                    description = "Action taken if the primary compaction model encounters an error."
                ) {
                    SubtleSegmentedControl(
                        options = listOf(
                            "api" to "API Fallback",
                            "cli" to "CLI Fallback",
                            "none" to "No Fallback"
                        ),
                        selected = current.fallbackStrategy,
                        onSelect = { current = current.copy(fallbackStrategy = it) }
                    )
                }
            }
        }

        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
            GradientButton(
                text = if (saved) "Compaction Settings Saved" else "Save Compaction Settings",
                onClick = {
                    onChange(current)
                    onSave()
                    saved = true
                },
                height = 34.dp
            )
        }
    }
}

@Composable
private fun PersonasTab(state: SettingsState, onIntent: (SettingsIntent) -> Unit) {
    val cc = LocalCcColors.current
    val clipboard = LocalClipboardManager.current
    val filePicker = remember { FilePicker() }

    var searchQuery by remember { mutableStateOf("") }
    var selectedCategory by remember { mutableStateOf("All") }
    var personaToDelete by remember { mutableStateOf<PredefinedPersona?>(null) }
    var viewingPersona by remember { mutableStateOf<PredefinedPersona?>(null) }
    var copiedExport by remember { mutableStateOf(false) }

    val currentViewingPersona = remember(viewingPersona?.id, state.personas) {
        viewingPersona?.id?.let { id -> state.personas.firstOrNull { it.id == id } } ?: viewingPersona
    }

    val categories = listOf(
        "All",
        "Software Engineering",
        "Scientific Research",
        "Writing & Journalism",
        "Product & Strategy",
        "Legal & Governance",
        "Healthcare & Medicine",
        "General Debate",
        "Custom"
    )

    val filteredPersonas = remember(state.personas, searchQuery, selectedCategory) {
        state.personas.filter { p ->
            val matchesCategory = when (selectedCategory) {
                "All" -> true
                "Custom" -> !p.isSystem
                else -> p.category.equals(selectedCategory, ignoreCase = true)
            }
            val matchesSearch = searchQuery.isBlank() ||
                p.name.contains(searchQuery, ignoreCase = true) ||
                p.role.contains(searchQuery, ignoreCase = true) ||
                p.category.contains(searchQuery, ignoreCase = true) ||
                p.description.contains(searchQuery, ignoreCase = true) ||
                p.systemPrompt.contains(searchQuery, ignoreCase = true)
            matchesCategory && matchesSearch
        }
    }

    if (currentViewingPersona != null) {
        // ── Detail Page for Selected Persona ──────────────────────────────────
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            // Back Button Bar
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically
            ) {
                IconButton(
                    onClick = { viewingPersona = null },
                    modifier = Modifier.size(36.dp)
                ) {
                    Icon(
                        Icons.AutoMirrored.Filled.ArrowBack,
                        contentDescription = "Back to personas",
                        tint = cc.textPrimary
                    )
                }
                Spacer(Modifier.width(8.dp))
                Text(
                    "Persona Details",
                    style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.Bold),
                    color = cc.textPrimary
                )
            }

            // Profile Card Header
            Surface(
                color = cc.panelAlt,
                shape = RoundedCornerShape(16.dp),
                border = BorderStroke(1.dp, cc.border),
                modifier = Modifier.fillMaxWidth()
            ) {
                Column(modifier = Modifier.padding(20.dp)) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(16.dp)
                    ) {
                        // Profile Avatar
                        Box(
                            modifier = Modifier
                                .size(64.dp)
                                .clip(CircleShape)
                                .background(cc.panelAlt)
                                .border(1.dp, cc.border, CircleShape),
                            contentAlignment = Alignment.Center
                        ) {
                            PersonaIconView(
                                icon = currentViewingPersona.icon,
                                category = currentViewingPersona.category,
                                size = 36.dp,
                                tint = cc.textMuted
                            )
                        }

                        Column(modifier = Modifier.weight(1f)) {
                            Text(
                                text = currentViewingPersona.name,
                                style = MaterialTheme.typography.titleLarge.copy(fontWeight = FontWeight.Bold),
                                color = cc.textPrimary
                            )
                            if (currentViewingPersona.role.isNotBlank()) {
                                Spacer(Modifier.height(2.dp))
                                Text(
                                    text = currentViewingPersona.role,
                                    style = MaterialTheme.typography.bodyMedium,
                                    color = cc.textMuted
                                )
                            }
                            Spacer(Modifier.height(8.dp))
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(8.dp)
                            ) {
                                // Area badge
                                Surface(
                                    color = cc.panelAlt,
                                    shape = RoundedCornerShape(6.dp),
                                    border = BorderStroke(0.75.dp, cc.border)
                                ) {
                                    Text(
                                        currentViewingPersona.category,
                                        style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.sp, fontWeight = FontWeight.Medium),
                                        color = cc.textMuted,
                                        modifier = Modifier.padding(horizontal = 8.dp, vertical = 3.dp)
                                    )
                                }

                                // Built-in / Custom badge
                                if (currentViewingPersona.isSystem) {
                                    Surface(
                                        color = cc.panel,
                                        border = BorderStroke(1.dp, cc.border.copy(alpha = 0.8f)),
                                        shape = RoundedCornerShape(6.dp)
                                    ) {
                                        Text(
                                            "Built-in",
                                            style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.sp, fontWeight = FontWeight.Medium),
                                            color = cc.textMuted,
                                            modifier = Modifier.padding(horizontal = 8.dp, vertical = 3.dp)
                                        )
                                    }
                                } else {
                                    Surface(
                                        color = Color(0xFF4CAF50).copy(alpha = 0.15f),
                                        border = BorderStroke(1.dp, Color(0xFF4CAF50).copy(alpha = 0.35f)),
                                        shape = RoundedCornerShape(6.dp)
                                    ) {
                                        Text(
                                            "Custom Persona",
                                            style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.sp, fontWeight = FontWeight.SemiBold),
                                            color = Color(0xFF4CAF50),
                                            modifier = Modifier.padding(horizontal = 8.dp, vertical = 3.dp)
                                        )
                                    }
                                }
                            }
                        }
                    }

                    Spacer(Modifier.height(16.dp))
                    HorizontalDivider(color = cc.border.copy(alpha = 0.35f))
                    Spacer(Modifier.height(16.dp))

                    // Edit & Actions Row
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(10.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        // Edit button to edit them
                        if (currentViewingPersona.isSystem) {
                            GradientButton(
                                text = "Customize / Copy",
                                icon = Icons.Outlined.ContentCopy,
                                onClick = { onIntent(SettingsIntent.EditPersona(currentViewingPersona.id)) },
                                height = 36.dp
                            )
                        } else {
                            GradientButton(
                                text = "Edit Persona",
                                icon = Icons.Filled.Edit,
                                onClick = { onIntent(SettingsIntent.EditPersona(currentViewingPersona.id)) },
                                height = 36.dp
                            )
                        }

                        OutlinedButton(
                            onClick = { onIntent(SettingsIntent.ExportSinglePersona(currentViewingPersona)) },
                            modifier = Modifier.height(40.dp)
                        ) {
                            Icon(Icons.Outlined.FileUpload, contentDescription = null, modifier = Modifier.size(16.dp))
                            Spacer(Modifier.width(6.dp))
                            Text("Export JSON", style = MaterialTheme.typography.labelMedium)
                        }

                        if (!currentViewingPersona.isSystem) {
                            OutlinedButton(
                                onClick = { personaToDelete = currentViewingPersona },
                                colors = ButtonDefaults.outlinedButtonColors(contentColor = MaterialTheme.colorScheme.error),
                                border = BorderStroke(1.dp, MaterialTheme.colorScheme.error.copy(alpha = 0.5f)),
                                modifier = Modifier.height(40.dp)
                            ) {
                                Icon(Icons.Filled.Delete, contentDescription = null, modifier = Modifier.size(16.dp))
                                Spacer(Modifier.width(6.dp))
                                Text("Delete", style = MaterialTheme.typography.labelMedium)
                            }
                        }
                    }
                }
            }

            // Description Section
            if (currentViewingPersona.description.isNotBlank()) {
                Surface(
                    color = cc.panelAlt,
                    shape = RoundedCornerShape(12.dp),
                    border = BorderStroke(1.dp, cc.border),
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Column(modifier = Modifier.padding(16.dp)) {
                        Text(
                            "Description & Specialty",
                            style = MaterialTheme.typography.titleSmall.copy(fontWeight = FontWeight.SemiBold),
                            color = cc.textPrimary
                        )
                        Spacer(Modifier.height(8.dp))
                        Text(
                            text = currentViewingPersona.description,
                            style = MaterialTheme.typography.bodyMedium.copy(lineHeight = 20.sp),
                            color = cc.textPrimary.copy(alpha = 0.9f)
                        )
                    }
                }
            }

            // Behavioral Modifiers Section
            if (currentViewingPersona.ponytail) {
                Surface(
                    color = cc.panelAlt,
                    shape = RoundedCornerShape(12.dp),
                    border = BorderStroke(1.dp, cc.border),
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Column(modifier = Modifier.padding(16.dp)) {
                        Text(
                            "Behavioral Modifiers",
                            style = MaterialTheme.typography.titleSmall.copy(fontWeight = FontWeight.SemiBold),
                            color = cc.textPrimary
                        )
                        Spacer(Modifier.height(8.dp))
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            Surface(
                                color = cc.panelAlt,
                                shape = RoundedCornerShape(6.dp),
                                border = BorderStroke(0.75.dp, cc.border)
                            ) {
                                Text(
                                    "Ponytail (Structured executive communication)",
                                    style = MaterialTheme.typography.bodySmall,
                                    color = cc.textPrimary,
                                    modifier = Modifier.padding(horizontal = 10.dp, vertical = 6.dp)
                                )
                            }
                        }
                    }
                }
            }

            // System Prompt Section
            Surface(
                color = cc.panelAlt,
                shape = RoundedCornerShape(12.dp),
                border = BorderStroke(1.dp, cc.border),
                modifier = Modifier.fillMaxWidth()
            ) {
                Column(modifier = Modifier.padding(16.dp)) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text(
                            "System Prompt & Guidelines",
                            style = MaterialTheme.typography.titleSmall.copy(fontWeight = FontWeight.SemiBold),
                            color = cc.textPrimary
                        )
                        TextButton(
                            onClick = { clipboard.setText(AnnotatedString(currentViewingPersona.systemPrompt)) },
                            contentPadding = PaddingValues(horizontal = 8.dp, vertical = 4.dp)
                        ) {
                            Icon(Icons.Outlined.ContentCopy, contentDescription = null, modifier = Modifier.size(14.dp), tint = cc.textMuted)
                            Spacer(Modifier.width(4.dp))
                            Text("Copy Prompt", style = MaterialTheme.typography.labelSmall, color = cc.textPrimary)
                        }
                    }
                    Spacer(Modifier.height(8.dp))
                    Surface(
                        color = cc.panel,
                        shape = RoundedCornerShape(8.dp),
                        border = BorderStroke(1.dp, cc.border.copy(alpha = 0.4f)),
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Text(
                            text = currentViewingPersona.systemPrompt,
                            style = MaterialTheme.typography.bodySmall.copy(
                                fontFamily = FontFamily.Monospace,
                                fontSize = 12.sp,
                                lineHeight = 18.sp
                            ),
                            color = cc.textPrimary.copy(alpha = 0.85f),
                            modifier = Modifier.padding(14.dp)
                        )
                    }
                }
            }
        }
    } else {
        // ── Profile Cards List View ──────────────────────────────────────────
        Column(
            modifier = Modifier.fillMaxSize(),
            verticalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            // ── Unified Header Toolbar ──
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                // Left: Search input + Category dropdown
                Row(
                    modifier = Modifier.weight(1f).padding(end = 12.dp),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    // Search Pill (34dp height)
                    Surface(
                        shape = RoundedCornerShape(10.dp),
                        color = cc.panelAlt,
                        border = BorderStroke(1.dp, cc.border.copy(alpha = 0.4f)),
                        modifier = Modifier.weight(1f).height(34.dp)
                    ) {
                        Row(
                            modifier = Modifier.fillMaxSize().padding(horizontal = 10.dp),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Icon(
                                Icons.Outlined.Search,
                                contentDescription = "Search",
                                tint = cc.textMuted,
                                modifier = Modifier.size(15.dp)
                            )
                            Spacer(Modifier.width(8.dp))
                            Box(modifier = Modifier.weight(1f), contentAlignment = Alignment.CenterStart) {
                                if (searchQuery.isEmpty()) {
                                    Text(
                                        "Search personas...",
                                        style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                                        color = cc.textMuted.copy(alpha = 0.7f)
                                    )
                                }
                                BasicTextField(
                                    value = searchQuery,
                                    onValueChange = { searchQuery = it },
                                    singleLine = true,
                                    textStyle = MaterialTheme.typography.bodyMedium.copy(
                                        color = cc.textPrimary,
                                        fontSize = 12.sp
                                    ),
                                    cursorBrush = SolidColor(cc.accent),
                                    modifier = Modifier.fillMaxWidth()
                                )
                            }
                            if (searchQuery.isNotEmpty()) {
                                IconButton(
                                    onClick = { searchQuery = "" },
                                    modifier = Modifier.size(18.dp)
                                ) {
                                    Icon(
                                        Icons.Default.Close,
                                        contentDescription = "Clear",
                                        tint = cc.textMuted,
                                        modifier = Modifier.size(11.dp)
                                    )
                                }
                            }
                        }
                    }

                    // Category Filter Dropdown
                    var categoryDropdownOpen by remember { mutableStateOf(false) }
                    val chevronRotation by animateFloatAsState(
                        targetValue = if (categoryDropdownOpen) 180f else 0f,
                        label = "CategoryChevronRotation"
                    )
                    Box {
                        Surface(
                            shape = RoundedCornerShape(10.dp),
                            color = cc.panelAlt,
                            border = BorderStroke(1.dp, cc.border.copy(alpha = 0.4f)),
                            modifier = Modifier.height(34.dp).clickable { categoryDropdownOpen = true }
                        ) {
                            Row(
                                modifier = Modifier.padding(horizontal = 10.dp),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(4.dp)
                            ) {
                                Text(
                                    text = if (selectedCategory == "All") "All Categories" else selectedCategory,
                                    style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.5.sp),
                                    color = if (selectedCategory == "All") cc.textMuted else cc.accent
                                )
                                Icon(
                                    Icons.Default.ArrowDropDown,
                                    contentDescription = null,
                                    modifier = Modifier
                                        .size(16.dp)
                                        .graphicsLayer { rotationZ = chevronRotation },
                                    tint = cc.textMuted
                                )
                            }
                        }

                        DropdownMenu(
                            expanded = categoryDropdownOpen,
                            onDismissRequest = { categoryDropdownOpen = false },
                            modifier = Modifier
                                .background(cc.panel)
                                .border(BorderStroke(1.dp, cc.border.copy(alpha = 0.4f)), RoundedCornerShape(10.dp))
                                .clip(RoundedCornerShape(10.dp))
                        ) {
                            categories.forEach { cat ->
                                val count = when (cat) {
                                    "All" -> state.personas.size
                                    "Custom" -> state.personas.count { !it.isSystem }
                                    else -> state.personas.count { it.category.equals(cat, ignoreCase = true) }
                                }
                                val isSelected = cat == selectedCategory
                                DropdownMenuItem(
                                    text = {
                                        Row(
                                            modifier = Modifier.fillMaxWidth(),
                                            horizontalArrangement = Arrangement.SpaceBetween,
                                            verticalAlignment = Alignment.CenterVertically
                                        ) {
                                            Text(
                                                cat,
                                                style = MaterialTheme.typography.bodySmall.copy(
                                                    fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal
                                                ),
                                                color = if (isSelected) cc.accent else cc.textPrimary
                                            )
                                            Spacer(Modifier.width(16.dp))
                                            Text(
                                                "($count)",
                                                style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.5.sp),
                                                color = if (isSelected) cc.accent.copy(alpha = 0.8f) else cc.textMuted
                                            )
                                        }
                                    },
                                    colors = MenuDefaults.itemColors(
                                        textColor = cc.textPrimary,
                                        leadingIconColor = cc.textMuted
                                    ),
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .background(if (isSelected) cc.panelAlt else Color.Transparent)
                                        .padding(horizontal = 4.dp),
                                    onClick = {
                                        selectedCategory = cat
                                        categoryDropdownOpen = false
                                    }
                                )
                            }
                        }
                    }
                }

                // Right: Action buttons
                Row(
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    ThemedTooltipBox("Browse Community Gallery") {
                        IconButton(
                            onClick = { onIntent(SettingsIntent.OpenGalleryDialog) },
                            modifier = Modifier.size(34.dp)
                        ) {
                            Icon(
                                Icons.Outlined.Language,
                                contentDescription = "Gallery",
                                tint = cc.accent,
                                modifier = Modifier.size(18.dp)
                            )
                        }
                    }

                    ThemedTooltipBox("Import personas JSON") {
                        IconButton(
                            onClick = { onIntent(SettingsIntent.OpenImportDialog) },
                            modifier = Modifier.size(34.dp)
                        ) {
                            Icon(
                                Icons.Outlined.FileDownload,
                                contentDescription = "Import",
                                tint = cc.textMuted,
                                modifier = Modifier.size(17.dp)
                            )
                        }
                    }

                    ThemedTooltipBox("Export all personas to JSON") {
                        IconButton(
                            onClick = { onIntent(SettingsIntent.ExportPersonas) },
                            modifier = Modifier.size(34.dp)
                        ) {
                            Icon(
                                Icons.Outlined.FileUpload,
                                contentDescription = "Export All",
                                tint = cc.textMuted,
                                modifier = Modifier.size(17.dp)
                            )
                        }
                    }

                    Button(
                        onClick = { onIntent(SettingsIntent.OpenPersonaBuilder) },
                        shape = RoundedCornerShape(8.dp),
                        contentPadding = PaddingValues(horizontal = 12.dp, vertical = 4.dp),
                        modifier = Modifier.height(34.dp)
                    ) {
                        Icon(Icons.Outlined.Add, contentDescription = null, modifier = Modifier.size(13.dp))
                        Spacer(Modifier.width(5.dp))
                        Text(
                            "New Persona",
                            style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.SemiBold),
                            color = MaterialTheme.colorScheme.onPrimary
                        )
                    }
                }
            }

            // ── Personas Content Area ──
            if (filteredPersonas.isEmpty()) {
                Box(
                    modifier = Modifier.fillMaxSize().padding(top = 40.dp),
                    contentAlignment = Alignment.TopCenter
                ) {
                    Column(horizontalAlignment = Alignment.CenterHorizontally) {
                        Icon(Icons.Outlined.Search, contentDescription = null, tint = cc.textMuted, modifier = Modifier.size(36.dp))
                        Spacer(Modifier.height(8.dp))
                        Text("No personas match your query", style = MaterialTheme.typography.bodyMedium, color = cc.textMuted)
                    }
                }
            } else {
                val customPersonas = remember(filteredPersonas) { filteredPersonas.filter { !it.isSystem } }
                val systemPersonas = remember(filteredPersonas) { filteredPersonas.filter { it.isSystem } }
                val isDefaultView = searchQuery.isBlank() && selectedCategory == "All"

                LazyVerticalGrid(
                    columns = GridCells.Adaptive(280.dp),
                    horizontalArrangement = Arrangement.spacedBy(14.dp),
                    verticalArrangement = Arrangement.spacedBy(14.dp),
                    contentPadding = PaddingValues(bottom = 24.dp),
                    modifier = Modifier.fillMaxSize()
                ) {
                    if (isDefaultView && customPersonas.isNotEmpty()) {
                        // Section: Custom Personas
                        item(span = { androidx.compose.foundation.lazy.grid.GridItemSpan(maxLineSpan) }) {
                            Text(
                                "My Personas (${customPersonas.size})",
                                style = MaterialTheme.typography.titleSmall.copy(
                                    fontWeight = FontWeight.SemiBold,
                                    fontSize = 13.sp
                                ),
                                color = cc.textPrimary,
                                modifier = Modifier.padding(top = 4.dp, bottom = 2.dp)
                            )
                        }

                        items(customPersonas, key = { it.id }) { persona ->
                            PersonaTile(persona = persona, cc = cc, onClick = { viewingPersona = persona })
                        }

                        // Section: Standard Library
                        item(span = { androidx.compose.foundation.lazy.grid.GridItemSpan(maxLineSpan) }) {
                            Text(
                                "Preset Library (${systemPersonas.size})",
                                style = MaterialTheme.typography.titleSmall.copy(
                                    fontWeight = FontWeight.SemiBold,
                                    fontSize = 13.sp
                                ),
                                color = cc.textPrimary,
                                modifier = Modifier.padding(top = 10.dp, bottom = 2.dp)
                            )
                        }

                        items(systemPersonas, key = { it.id }) { persona ->
                            PersonaTile(persona = persona, cc = cc, onClick = { viewingPersona = persona })
                        }
                    } else {
                        items(filteredPersonas, key = { it.id }) { persona ->
                            PersonaTile(persona = persona, cc = cc, onClick = { viewingPersona = persona })
                        }
                    }
                }
            }
        }
    }

    // ── Dialog: Import Personas ───────────────────────────────────────────────
    if (state.showImportDialog) {
        var importText by remember { mutableStateOf("") }
        val pickImportFile = filePicker.registerPicker { _, content ->
            importText = content
        }

        AlertDialog(
            onDismissRequest = { onIntent(SettingsIntent.DismissImportDialog) },
            title = { Text("Import Personas") },
            text = {
                Column(Modifier.fillMaxWidth()) {
                    Text(
                        "Import personas from a JSON array file or paste JSON directly below.",
                        style = MaterialTheme.typography.bodySmall,
                        color = cc.textMuted
                    )
                    Spacer(Modifier.height(12.dp))
                    OutlinedButton(
                        onClick = pickImportFile,
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Icon(Icons.Outlined.FileOpen, contentDescription = null, modifier = Modifier.size(16.dp))
                        Spacer(Modifier.width(8.dp))
                        Text("Choose JSON File...")
                    }
                    Spacer(Modifier.height(10.dp))
                    OutlinedTextField(
                        value = importText,
                        onValueChange = { importText = it },
                        placeholder = { Text("Paste persona JSON array here...") },
                        minLines = 8,
                        maxLines = 14,
                        modifier = Modifier.fillMaxWidth()
                    )
                }
            },
            confirmButton = {
                GradientButton(
                    text = "Import",
                    onClick = { onIntent(SettingsIntent.ImportPersonas(importText)) },
                    enabled = importText.isNotBlank()
                )
            },
            dismissButton = {
                TextButton(onClick = { onIntent(SettingsIntent.DismissImportDialog) }) {
                    Text("Cancel")
                }
            }
        )
    }

    // ── Dialog: Export Personas ───────────────────────────────────────────────
    state.exportJsonDialogContent?.let { jsonContent ->
        AlertDialog(
            onDismissRequest = {
                copiedExport = false
                onIntent(SettingsIntent.DismissExportDialog)
            },
            title = { Text("Export Personas (JSON)") },
            text = {
                Column(Modifier.fillMaxWidth()) {
                    Text(
                        "Here is your exported JSON. You can copy it or save it to a file.",
                        style = MaterialTheme.typography.bodySmall,
                        color = cc.textMuted
                    )
                    Spacer(Modifier.height(10.dp))
                    OutlinedTextField(
                        value = jsonContent,
                        onValueChange = {},
                        readOnly = true,
                        minLines = 8,
                        maxLines = 14,
                        modifier = Modifier.fillMaxWidth()
                    )
                }
            },
            confirmButton = {
                GradientButton(
                    text = if (copiedExport) "Copied!" else "Copy to Clipboard",
                    icon = Icons.Default.ContentCopy,
                    onClick = {
                        clipboard.setText(AnnotatedString(jsonContent))
                        copiedExport = true
                    }
                )
            },
            dismissButton = {
                TextButton(
                    onClick = {
                        copiedExport = false
                        onIntent(SettingsIntent.DismissExportDialog)
                    }
                ) {
                    Text("Close")
                }
            }
        )
    }

    // ── Dialog: Delete Confirmation ───────────────────────────────────────────
    personaToDelete?.let { target ->
        AlertDialog(
            onDismissRequest = { personaToDelete = null },
            title = { Text("Delete Persona") },
            text = {
                Text("Are you sure you want to permanently delete \"${target.name}\"? This action cannot be undone.")
            },
            confirmButton = {
                Button(
                    onClick = {
                        onIntent(SettingsIntent.DeletePersona(target.id))
                        if (viewingPersona?.id == target.id) viewingPersona = null
                        personaToDelete = null
                    },
                    colors = ButtonDefaults.buttonColors(containerColor = MaterialTheme.colorScheme.error)
                ) {
                    Text("Delete", color = MaterialTheme.colorScheme.onError)
                }
            },
            dismissButton = {
                TextButton(onClick = { personaToDelete = null }) {
                    Text("Cancel")
                }
            }
        )
    }

    // ── Dialog: Community Persona Gallery ─────────────────────────────────────
    if (state.showGalleryDialog) {
        val installedIds = remember(state.personas) { state.personas.map { it.id }.toSet() }
        com.dialex.presentation.settings.personas.gallery.PersonaGalleryDialog(
            installedPersonaIds = installedIds,
            onInstallPersona = { persona ->
                onIntent(SettingsIntent.InstallGalleryPersona(persona))
            },
            onDismiss = { onIntent(SettingsIntent.DismissGalleryDialog) }
        )
    }
}

@Composable
private fun PersonaTile(
    persona: PredefinedPersona,
    cc: com.dialex.theme.CcPalette,
    onClick: () -> Unit
) {
    Card(
        colors = CardDefaults.cardColors(containerColor = cc.panel),
        border = BorderStroke(1.dp, cc.border.copy(alpha = 0.35f)),
        shape = RoundedCornerShape(12.dp),
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .clickable(onClick = onClick)
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(9.dp)
        ) {
            // Top Row: Avatar + Name / Role + Badge
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(11.dp)
            ) {
                Box(
                    modifier = Modifier
                        .size(38.dp)
                        .clip(CircleShape)
                        .background(cc.panelAlt)
                        .border(0.75.dp, cc.border.copy(alpha = 0.6f), CircleShape),
                    contentAlignment = Alignment.Center
                ) {
                    PersonaIconView(
                        icon = persona.icon,
                        category = persona.category,
                        size = 20.dp,
                        tint = cc.textMuted
                    )
                }

                Column(modifier = Modifier.weight(1f)) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text(
                            text = persona.name,
                            style = MaterialTheme.typography.bodyMedium.copy(
                                fontWeight = FontWeight.SemiBold,
                                fontSize = 13.5.sp
                            ),
                            color = cc.textPrimary,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            modifier = Modifier.weight(1f, fill = false)
                        )

                        if (!persona.isSystem) {
                            Spacer(Modifier.width(6.dp))
                            Surface(
                                color = cc.accent.copy(alpha = 0.12f),
                                border = BorderStroke(0.75.dp, cc.accent.copy(alpha = 0.35f)),
                                shape = RoundedCornerShape(4.dp)
                            ) {
                                Text(
                                    "Custom",
                                    style = MaterialTheme.typography.labelSmall.copy(fontSize = 9.5.sp, fontWeight = FontWeight.SemiBold),
                                    color = cc.accent,
                                    modifier = Modifier.padding(horizontal = 5.dp, vertical = 1.5.dp)
                                )
                            }
                        }
                    }

                    if (persona.role.isNotBlank()) {
                        Spacer(Modifier.height(1.dp))
                        Text(
                            text = persona.role,
                            style = MaterialTheme.typography.bodySmall.copy(
                                fontSize = 11.5.sp,
                                fontWeight = FontWeight.Normal
                            ),
                            color = cc.textMuted,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                    }
                }
            }

            // Description snippet
            if (persona.description.isNotBlank()) {
                Text(
                    text = persona.description,
                    style = MaterialTheme.typography.bodySmall.copy(
                        fontSize = 11.5.sp,
                        lineHeight = 16.sp
                    ),
                    color = cc.textMuted.copy(alpha = 0.85f),
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis
                )
            }

            // Bottom Row: Category Tag + Modifiers + Chevron Indicator
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(5.dp),
                    modifier = Modifier.weight(1f, fill = false)
                ) {
                    Surface(
                        color = cc.panelAlt,
                        shape = RoundedCornerShape(4.dp),
                        border = BorderStroke(0.75.dp, cc.border.copy(alpha = 0.35f))
                    ) {
                        Text(
                            text = persona.category,
                            style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.sp),
                            color = cc.textMuted,
                            modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp)
                        )
                    }

                    if (persona.ponytail) {
                        Surface(
                            color = cc.agentThird.copy(alpha = 0.12f),
                            shape = RoundedCornerShape(4.dp)
                        ) {
                            Text(
                                "Ponytail",
                                style = MaterialTheme.typography.labelSmall.copy(fontSize = 9.5.sp, fontWeight = FontWeight.Medium),
                                color = cc.agentThird,
                                modifier = Modifier.padding(horizontal = 5.dp, vertical = 2.dp)
                            )
                        }
                    }
                }

                Icon(
                    Icons.AutoMirrored.Filled.KeyboardArrowRight,
                    contentDescription = "View Profile",
                    tint = cc.textMuted.copy(alpha = 0.5f),
                    modifier = Modifier.size(15.dp)
                )
            }
        }
    }
}

@Composable
private fun LimitsTab(budget: Int, onChange: (Int) -> Unit, onSave: () -> Unit) {
    val cc = LocalCcColors.current
    var text by remember(budget) { mutableStateOf(budget.toString()) }
    var saved by remember { mutableStateOf(false) }

    LaunchedEffect(saved) {
        if (saved) {
            delay(2000)
            saved = false
        }
    }

    val presets = listOf(100_000, 200_000, 500_000, 1_000_000, 2_000_000, 5_000_000)

    Column(
        modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(20.dp)
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(
                "Discussion Token Safety",
                style = MaterialTheme.typography.titleMedium.copy(
                    fontWeight = FontWeight.SemiBold,
                    fontSize = 14.sp
                ),
                color = cc.textPrimary
            )
            Text(
                "Configure token consumption safety boundaries to prevent unbounded usage during complex or recursive debate rounds.",
                style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                color = cc.textMuted
            )
        }

        SettingCard {
            SettingRow(
                title = "Max Tokens per Discussion",
                description = "Maximum cumulative tokens allocated before triggering limit safety action (0 disables limit)."
            ) {
                OutlinedTextField(
                    value = text,
                    onValueChange = { if (it.all(Char::isDigit)) text = it },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    textStyle = MaterialTheme.typography.bodyMedium.copy(fontFamily = FontFamily.Monospace, fontSize = 13.sp),
                    modifier = Modifier.width(160.dp),
                    shape = RoundedCornerShape(8.dp),
                    trailingIcon = {
                        Text("tokens", style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.sp), color = cc.textMuted, modifier = Modifier.padding(end = 8.dp))
                    }
                )
            }

            HorizontalDivider(color = cc.border.copy(alpha = 0.25f), thickness = 0.75.dp)

            SettingRow(
                title = "Preset Safety Thresholds",
                description = "Quick-select predefined token safety budgets."
            ) {
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    presets.forEach { preset ->
                        val isSelected = text.toIntOrNull() == preset
                        Surface(
                            shape = RoundedCornerShape(6.dp),
                            color = if (isSelected) cc.panel else cc.panelAlt,
                            border = BorderStroke(0.75.dp, if (isSelected) cc.border.copy(alpha = 0.8f) else cc.border.copy(alpha = 0.35f)),
                            modifier = Modifier
                                .clip(RoundedCornerShape(6.dp))
                                .clickable { text = preset.toString() }
                        ) {
                            Text(
                                when (preset) {
                                    100_000 -> "100K"
                                    200_000 -> "200K"
                                    500_000 -> "500K"
                                    1_000_000 -> "1M (Default)"
                                    2_000_000 -> "2M"
                                    5_000_000 -> "5M"
                                    else -> "${preset / 1000}K"
                                },
                                style = MaterialTheme.typography.labelSmall.copy(
                                    fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal,
                                    fontSize = 11.5.sp
                                ),
                                color = if (isSelected) cc.textPrimary else cc.textMuted,
                                modifier = Modifier.padding(horizontal = 8.dp, vertical = 5.dp)
                            )
                        }
                    }
                }
            }
        }

        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
            GradientButton(
                text = if (saved) "Budget Saved" else "Save Token Budget",
                onClick = {
                    onChange(text.toIntOrNull() ?: 1_000_000)
                    onSave()
                    saved = true
                },
                height = 34.dp
            )
        }
    }
}

@Composable
private fun ConnectionTab(
    connectionLabel: String,
    onSwitch: () -> Unit,
    onNavigateToLogs: () -> Unit
) {
    val cc = LocalCcColors.current
    val isLocal = connectionLabel.contains("device", ignoreCase = true) || connectionLabel.isBlank()
    val allLogs by AppLogStore.logs.collectAsState()

    val recentEngineLogs = remember(allLogs) {
        allLogs.takeLast(10).filter { it.tag.equals("EngineRPC", ignoreCase = true) }
    }
    val hasConnectionIssue = remember(recentEngineLogs) {
        recentEngineLogs.isNotEmpty() && recentEngineLogs.last().level != AppLogLevel.INFO
    }
    val lastErrorMsg = remember(recentEngineLogs) {
        recentEngineLogs.lastOrNull { it.level != AppLogLevel.INFO }?.message
    }

    Column(
        modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(20.dp)
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(
                "Engine Connection & Telemetry",
                style = MaterialTheme.typography.titleMedium.copy(
                    fontWeight = FontWeight.SemiBold,
                    fontSize = 14.sp
                ),
                color = cc.textPrimary
            )
            Text(
                "Real-time status and endpoint of the backend engine executing debates, agent tool calls, and model requests.",
                style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                color = cc.textMuted
            )
        }

        SettingCard {
            SettingRow(
                title = "Engine Deployment Type",
                description = if (isLocal) "Running directly on this local machine as a background loopback service." else "Connected over network to a remote Dialex engine server."
            ) {
                Surface(
                    shape = RoundedCornerShape(6.dp),
                    color = cc.panelAlt,
                    border = BorderStroke(0.75.dp, cc.border)
                ) {
                    Text(
                        if (isLocal) "Local Background Engine" else "Remote Server Host",
                        style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.Medium, fontSize = 11.5.sp),
                        color = cc.textMuted,
                        modifier = Modifier.padding(horizontal = 10.dp, vertical = 4.dp)
                    )
                }
            }

            HorizontalDivider(color = cc.border.copy(alpha = 0.25f), thickness = 0.75.dp)

            SettingRow(
                title = "Connection Status",
                description = if (hasConnectionIssue) (lastErrorMsg ?: "Connection refused — engine daemon offline or starting up") else "Current connectivity and responsiveness to local or remote RPCs."
            ) {
                if (hasConnectionIssue) {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(6.dp),
                        modifier = Modifier
                            .clip(RoundedCornerShape(6.dp))
                            .background(Color(0xFFF59E0B).copy(alpha = 0.12f))
                            .border(BorderStroke(0.75.dp, Color(0xFFF59E0B).copy(alpha = 0.35f)), RoundedCornerShape(6.dp))
                            .padding(horizontal = 10.dp, vertical = 4.dp)
                    ) {
                        Box(Modifier.size(7.dp).clip(CircleShape).background(Color(0xFFF59E0B)))
                        Text(
                            "Offline / Retrying",
                            style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.SemiBold, fontSize = 11.5.sp),
                            color = Color(0xFFF59E0B)
                        )
                    }
                } else {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(6.dp),
                        modifier = Modifier
                            .clip(RoundedCornerShape(6.dp))
                            .background(Color(0xFF4CAF50).copy(alpha = 0.12f))
                            .border(BorderStroke(0.75.dp, Color(0xFF4CAF50).copy(alpha = 0.35f)), RoundedCornerShape(6.dp))
                            .padding(horizontal = 10.dp, vertical = 4.dp)
                    ) {
                        Box(Modifier.size(7.dp).clip(CircleShape).background(Color(0xFF4CAF50)))
                        Text(
                            "Active & Connected",
                            style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.SemiBold, fontSize = 11.5.sp),
                            color = Color(0xFF4CAF50)
                        )
                    }
                }
            }

            HorizontalDivider(color = cc.border.copy(alpha = 0.25f), thickness = 0.75.dp)

            SettingRow(
                title = "Endpoint URL",
                description = "Network address utilized by Dialex frontend clients."
            ) {
                Text(
                    if (isLocal) "http://127.0.0.1:7890 (Loopback)" else connectionLabel,
                    style = MaterialTheme.typography.bodySmall.copy(fontFamily = FontFamily.Monospace, fontSize = 12.sp),
                    color = cc.textPrimary
                )
            }

            HorizontalDivider(color = cc.border.copy(alpha = 0.25f), thickness = 0.75.dp)

            SettingRow(
                title = "Live Diagnostics & Logs",
                description = "Inspect real-time network payloads, RPC latency, and daemon errors."
            ) {
                OutlinedButton(
                    onClick = onNavigateToLogs,
                    shape = RoundedCornerShape(8.dp),
                    contentPadding = PaddingValues(horizontal = 12.dp, vertical = 4.dp),
                    modifier = Modifier.height(32.dp)
                ) {
                    Icon(Icons.Outlined.Speed, contentDescription = null, modifier = Modifier.size(13.dp))
                    Spacer(Modifier.width(6.dp))
                    Text("View Logs & Events", style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.5.sp))
                }
            }
        }

        SettingCard {
            SettingRow(
                title = "Switch Engine Instance",
                description = "Disconnect from this engine and switch to a different local or remote Dialex server."
            ) {
                OutlinedButton(
                    onClick = onSwitch,
                    shape = RoundedCornerShape(8.dp),
                    contentPadding = PaddingValues(horizontal = 16.dp, vertical = 6.dp),
                    modifier = Modifier.height(34.dp)
                ) {
                    Text("Switch Connection…", style = MaterialTheme.typography.labelMedium.copy(fontSize = 12.sp))
                }
            }
        }
    }
}

@Composable
private fun LogsTab() {
    ServerLogsViewer(modifier = Modifier.fillMaxWidth())
}

@Composable
private fun DebatePolicyTab(
    policy: DebatePolicy,
    onPolicyChange: (DebatePolicy) -> Unit,
    onSave: () -> Unit
) {
    val cc = LocalCcColors.current

    Column(
        modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(16.dp)
    ) {
        // Human-Like Conversational Dialogue (Anti-Fluff)
        SettingCard {
            SettingRow(
                title = "Human-Like Conversational Dialogue",
                description = "Forces concise, natural 2–4 sentence turns without robotic pleasantries, headers, or essay monologues."
            ) {
                AestheticSwitch(
                    checked = policy.humanDialogueMode,
                    onCheckedChange = { onPolicyChange(policy.copy(humanDialogueMode = it)) }
                )
            }

            HorizontalDivider(color = cc.border.copy(alpha = 0.25f), thickness = 0.75.dp)

            Column(
                modifier = Modifier.fillMaxWidth().padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Column(modifier = Modifier.weight(1f)) {
                        Text(
                            "Anti-Fluff Directive (Human Dialogue Prompt)",
                            style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.Medium),
                            color = cc.textPrimary
                        )
                        Text(
                            "Configurable in Global Settings only. Injected into participants' prompts when dialogue mode is enabled.",
                            style = MaterialTheme.typography.labelSmall,
                            color = cc.textMuted
                        )
                    }
                    if (policy.humanDialogueDirective.isNotBlank() && policy.humanDialogueDirective != com.dialex.model.DEFAULT_HUMAN_DIALOGUE_DIRECTIVE) {
                        Text(
                            "Reset to Default",
                            style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.SemiBold),
                            color = cc.accent,
                            modifier = Modifier
                                .clip(RoundedCornerShape(4.dp))
                                .clickable {
                                    onPolicyChange(policy.copy(humanDialogueDirective = ""))
                                }
                                .padding(horizontal = 6.dp, vertical = 2.dp)
                        )
                    }
                }

                val currentHumanText = policy.humanDialogueDirective.ifBlank { com.dialex.model.DEFAULT_HUMAN_DIALOGUE_DIRECTIVE }
                SubtleTextArea(
                    value = currentHumanText,
                    onValueChange = { onPolicyChange(policy.copy(humanDialogueDirective = it)) },
                    minHeight = 80.dp,
                    maxLines = 6,
                    placeholder = "Enter custom anti-fluff directive..."
                )
            }
        }

        // Autopilot & Human Intervention Mode
        SettingCard {
            SettingRow(
                title = "Autonomous Autopilot & User Participation",
                description = "Choose whether discussions run fully autonomously without user input or require interactive human steering."
            )

            HorizontalDivider(color = cc.border.copy(alpha = 0.25f), thickness = 0.75.dp)

            Column(
                modifier = Modifier.fillMaxWidth().padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                UserInterventionPolicy.entries.forEach { mode ->
                    val isSelected = policy.userInterventionPolicy == mode
                    Surface(
                        shape = RoundedCornerShape(10.dp),
                        color = if (isSelected) cc.accent.copy(alpha = 0.10f) else cc.panelAlt,
                        border = BorderStroke(1.dp, if (isSelected) cc.accent else cc.border.copy(alpha = 0.5f)),
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(10.dp))
                            .clickable { onPolicyChange(policy.copy(userInterventionPolicy = mode)) }
                    ) {
                        Row(
                            modifier = Modifier.fillMaxWidth().padding(14.dp),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Box(
                                modifier = Modifier
                                    .size(20.dp)
                                    .clip(CircleShape)
                                    .border(1.5.dp, if (isSelected) cc.accent else cc.textMuted, CircleShape)
                                    .padding(3.5.dp)
                            ) {
                                if (isSelected) {
                                    Box(modifier = Modifier.fillMaxSize().clip(CircleShape).background(cc.accent))
                                }
                            }
                            Spacer(Modifier.width(12.dp))
                            Column(modifier = Modifier.weight(1f)) {
                                Text(
                                    mode.label,
                                    style = MaterialTheme.typography.bodyMedium.copy(
                                        fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Medium
                                    ),
                                    color = if (isSelected) cc.accent else cc.textPrimary
                                )
                                Spacer(Modifier.height(2.dp))
                                Text(
                                    mode.description,
                                    style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                                    color = cc.textMuted
                                )
                            }
                        }
                    }
                }
            }
        }

        // Shared Memory Strategy Defaults
        val mem = policy.sharedMemory
        SettingCard {
            SettingRow(
                title = "Shared Memory Compression",
                description = "Default rolling knowledge summary strategy inherited by new projects and debates."
            ) {
                AestheticSwitch(
                    checked = mem.enabled,
                    onCheckedChange = { onPolicyChange(policy.copy(sharedMemory = mem.copy(enabled = it))) }
                )
            }

            if (mem.enabled) {
                HorizontalDivider(color = cc.border.copy(alpha = 0.25f), thickness = 0.75.dp)
                Column(
                    modifier = Modifier.fillMaxWidth().padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(14.dp)
                ) {
                    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceBetween,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Text("Max Summary Tokens Limit", style = MaterialTheme.typography.bodyMedium, color = cc.textPrimary)
                            Text("${mem.maxSummaryTokens} tokens", style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.SemiBold), color = cc.textPrimary)
                        }
                        AestheticSlider(
                            value = mem.maxSummaryTokens.toFloat().coerceIn(300f, 10000f),
                            onValueChange = { onPolicyChange(policy.copy(sharedMemory = mem.copy(maxSummaryTokens = it.toInt()))) },
                            valueRange = 300f..10000f,
                            modifier = Modifier.fillMaxWidth().height(22.dp)
                        )
                        Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                            listOf(600 to "600", 1500 to "1.5k", 3000 to "3k", 6000 to "6k", 10000 to "10k (Max)").forEach { (tok, lbl) ->
                                val isSel = mem.maxSummaryTokens == tok
                                Surface(
                                    shape = RoundedCornerShape(4.dp),
                                    color = if (isSel) cc.accent.copy(alpha = 0.15f) else cc.panelAlt,
                                    border = BorderStroke(0.5.dp, if (isSel) cc.accent else cc.border.copy(alpha = 0.4f)),
                                    modifier = Modifier.clip(RoundedCornerShape(4.dp)).clickable {
                                        onPolicyChange(policy.copy(sharedMemory = mem.copy(maxSummaryTokens = tok)))
                                    }
                                ) {
                                    Text(
                                        lbl,
                                        style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.5.sp, fontWeight = if (isSel) FontWeight.SemiBold else FontWeight.Normal),
                                        color = if (isSel) cc.accent else cc.textMuted,
                                        modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.5.dp)
                                    )
                                }
                            }
                        }
                    }

                    Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
                        Column(modifier = Modifier.weight(1f)) {
                            Text("Anchor Round 1 Verbatim", style = MaterialTheme.typography.bodyMedium, color = cc.textPrimary)
                            Text("Always pass opening turns in full for foundational grounding.", style = MaterialTheme.typography.bodySmall, color = cc.textMuted)
                        }
                        AestheticSwitch(checked = mem.includeFullRound1, onCheckedChange = { onPolicyChange(policy.copy(sharedMemory = mem.copy(includeFullRound1 = it))) })
                    }

                    Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
                        Column(modifier = Modifier.weight(1f)) {
                            Text("Include Own Prior Turn", style = MaterialTheme.typography.bodyMedium, color = cc.textPrimary)
                            Text("Preserves each agent's individual conversational memory.", style = MaterialTheme.typography.bodySmall, color = cc.textMuted)
                        }
                        AestheticSwitch(checked = mem.includeOwnLastTurn, onCheckedChange = { onPolicyChange(policy.copy(sharedMemory = mem.copy(includeOwnLastTurn = it))) })
                    }
                }
            }
        }

        // Deliberation Depth Defaults
        val depth = policy.depth
        SettingCard {
            SettingRow(
                title = "Deliberation Depth & Cognitive Load",
                description = "Default language complexity, analytical rigor, and turn length policy across discussions."
            ) {
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    listOf(
                        com.dialex.model.DepthMode.CASUAL to "Casual",
                        com.dialex.model.DepthMode.EXECUTIVE to "Executive",
                        com.dialex.model.DepthMode.ACADEMIC to "Academic"
                    ).forEach { (mode, label) ->
                        val isSelected = depth.mode == mode
                        Surface(
                            shape = RoundedCornerShape(6.dp),
                            color = if (isSelected) cc.accent.copy(alpha = 0.15f) else cc.panelAlt,
                            border = BorderStroke(0.75.dp, if (isSelected) cc.accent else cc.border.copy(alpha = 0.4f)),
                            modifier = Modifier.clip(RoundedCornerShape(6.dp)).clickable {
                                onPolicyChange(policy.copy(depth = com.dialex.model.DepthConfig.preset(mode)))
                            }
                        ) {
                            Text(
                                label,
                                style = MaterialTheme.typography.labelSmall.copy(
                                    fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal
                                ),
                                color = if (isSelected) cc.accent else cc.textPrimary,
                                modifier = Modifier.padding(horizontal = 10.dp, vertical = 6.dp)
                            )
                        }
                    }
                }
            }
        }

        // Moderator Agent & Dialectic Steerage Defaults
        val mod = policy.moderation
        SettingCard {
            SettingRow(
                title = "Dynamic Moderator & Dialectic Steerage",
                description = "Active AI Moderator Chair to detect semantic drift, inject checkpoints, and enforce convergence."
            ) {
                AestheticSwitch(
                    checked = mod.enabled,
                    onCheckedChange = { onPolicyChange(policy.copy(moderation = mod.copy(enabled = it))) }
                )
            }

            if (mod.enabled) {
                HorizontalDivider(color = cc.border.copy(alpha = 0.25f), thickness = 0.75.dp)
                Column(
                    modifier = Modifier.fillMaxWidth().padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(14.dp)
                ) {
                    // Style Selection
                    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                        Text("Moderation Style", style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.Medium), color = cc.textPrimary)
                        Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                            listOf(
                                com.dialex.model.ModerationStyle.DYNAMIC_ACTIVE_STEERAGE to "Dynamic Drift",
                                com.dialex.model.ModerationStyle.PERIODIC_CHECKPOINT to "Checkpoints",
                                com.dialex.model.ModerationStyle.STRICT_ARBITRATION to "Strict Arbiter",
                                com.dialex.model.ModerationStyle.PASSIVE_WRAPUP_ONLY to "Wrap-Up Only"
                            ).forEach { (st, label) ->
                                val isSelected = mod.style == st
                                Surface(
                                    shape = RoundedCornerShape(6.dp),
                                    color = if (isSelected) cc.accent.copy(alpha = 0.12f) else cc.panelAlt,
                                    border = BorderStroke(0.75.dp, if (isSelected) cc.accent else cc.border.copy(alpha = 0.45f)),
                                    modifier = Modifier.weight(1f).clip(RoundedCornerShape(6.dp)).clickable {
                                        onPolicyChange(policy.copy(moderation = mod.copy(style = st)))
                                    }
                                ) {
                                    Box(modifier = Modifier.padding(vertical = 6.dp, horizontal = 4.dp), contentAlignment = Alignment.Center) {
                                        Text(
                                            label,
                                            style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.sp, fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal),
                                            color = if (isSelected) cc.accent else cc.textPrimary,
                                            maxLines = 1
                                        )
                                    }
                                }
                            }
                        }
                    }

                    // Persona Selection
                    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                        Text("Moderator Persona", style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.Medium), color = cc.textPrimary)
                        Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                            listOf(
                                com.dialex.model.ModeratorPersona.DELIBERATION_CHAIR to "Deliberation Chair",
                                com.dialex.model.ModeratorPersona.EXECUTIVE_ARBITER to "Executive Arbiter",
                                com.dialex.model.ModeratorPersona.SOCRATIC_PROBE to "Socratic Inquirer",
                                com.dialex.model.ModeratorPersona.DEVILS_ADVOCATE_CHAIR to "Devil's Advocate"
                            ).forEach { (pers, label) ->
                                val isSelected = mod.persona == pers
                                Surface(
                                    shape = RoundedCornerShape(6.dp),
                                    color = if (isSelected) cc.accent.copy(alpha = 0.12f) else cc.panelAlt,
                                    border = BorderStroke(0.75.dp, if (isSelected) cc.accent else cc.border.copy(alpha = 0.45f)),
                                    modifier = Modifier.weight(1f).clip(RoundedCornerShape(6.dp)).clickable {
                                        onPolicyChange(policy.copy(moderation = mod.copy(persona = pers)))
                                    }
                                ) {
                                    Box(modifier = Modifier.padding(vertical = 6.dp, horizontal = 4.dp), contentAlignment = Alignment.Center) {
                                        Text(
                                            label,
                                            style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.5.sp, fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal),
                                            color = if (isSelected) cc.accent else cc.textPrimary,
                                            maxLines = 1
                                        )
                                    }
                                }
                            }
                        }
                    }

                    // Drift Sensitivity Slider
                    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceBetween,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Text("Topic Drift Sensitivity Threshold", style = MaterialTheme.typography.bodyMedium, color = cc.textPrimary)
                            Text("${(mod.driftThreshold * 100).toInt()}% minimum topicality", style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.SemiBold), color = cc.textPrimary)
                        }
                        AestheticSlider(
                            value = mod.driftThreshold.toFloat().coerceIn(0.2f, 0.8f),
                            onValueChange = {
                                onPolicyChange(policy.copy(moderation = mod.copy(driftThreshold = (it * 100).roundToInt() / 100.0)))
                            },
                            valueRange = 0.2f..0.8f,
                            modifier = Modifier.fillMaxWidth().height(22.dp)
                        )
                    }

                    // Steerage Directives Enforcement
                    Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
                        Column(modifier = Modifier.weight(1f)) {
                            Text("Enforce Steerage Directives in Prompts", style = MaterialTheme.typography.bodySmall, color = cc.textPrimary)
                            Text("Injects moderator directives into next agent turns as mandatory context.", style = MaterialTheme.typography.labelSmall, color = cc.textMuted)
                        }
                        AestheticSwitch(
                            checked = mod.enforceSteerageDirectives,
                            onCheckedChange = { onPolicyChange(policy.copy(moderation = mod.copy(enforceSteerageDirectives = it))) }
                        )
                    }

                    // Topic Drift Guardrail (Anti-Rabbit-Hole)
                    HorizontalDivider(color = cc.border.copy(alpha = 0.25f), thickness = 0.75.dp)
                    Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
                        Column(modifier = Modifier.weight(1f)) {
                            Text("Topic Drift Guardrail (Anti-Rabbit-Hole)", style = MaterialTheme.typography.bodySmall.copy(fontWeight = FontWeight.Medium), color = cc.textPrimary)
                            Text("Keeps participants strictly anchored to the core question without digressing into tangents or rabbit holes.", style = MaterialTheme.typography.labelSmall, color = cc.textMuted)
                        }
                        AestheticSwitch(
                            checked = mod.detectTopicDrift,
                            onCheckedChange = { onPolicyChange(policy.copy(moderation = mod.copy(detectTopicDrift = it))) }
                        )
                    }

                    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceBetween,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Column(modifier = Modifier.weight(1f)) {
                                Text("Anti-Rabbit-Hole Directive", style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.Medium), color = cc.textPrimary)
                                Text("Configurable in Global Settings only. Injected as a mandatory focus constraint when topic drift guardrail is enabled.", style = MaterialTheme.typography.labelSmall, color = cc.textMuted)
                            }
                            if (mod.topicDriftDirective.isNotBlank() && mod.topicDriftDirective != com.dialex.model.DEFAULT_TOPIC_DRIFT_DIRECTIVE) {
                                Text(
                                    "Reset to Default",
                                    style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.SemiBold),
                                    color = cc.accent,
                                    modifier = Modifier
                                        .clip(RoundedCornerShape(4.dp))
                                        .clickable {
                                            onPolicyChange(policy.copy(moderation = mod.copy(topicDriftDirective = "")))
                                        }
                                        .padding(horizontal = 6.dp, vertical = 2.dp)
                                )
                            }
                        }
                        val currentDriftText = mod.topicDriftDirective.ifBlank { com.dialex.model.DEFAULT_TOPIC_DRIFT_DIRECTIVE }
                        SubtleTextArea(
                            value = currentDriftText,
                            onValueChange = { onPolicyChange(policy.copy(moderation = mod.copy(topicDriftDirective = it))) },
                            minHeight = 80.dp,
                            maxLines = 6,
                            placeholder = "Enter custom anti-rabbit-hole directive..."
                        )
                    }
                }
            }
        }

        // Deliverable & Auto-Save Defaults
        val del = policy.deliverable
        val out = policy.output
        SettingCard {
            SettingRow(
                title = "Deliverable & Auto-Save Defaults",
                description = "Default outcome artifact format and file persistence behavior."
            )

            HorizontalDivider(color = cc.border.copy(alpha = 0.25f), thickness = 0.75.dp)

            Column(
                modifier = Modifier.fillMaxWidth().padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                Text("Default Deliverable Format", style = MaterialTheme.typography.bodyMedium, color = cc.textPrimary)
                Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    DeliverableFormat.entries.take(4).forEach { fmt ->
                        val isSel = del.format == fmt
                        Surface(
                            shape = RoundedCornerShape(6.dp),
                            color = if (isSel) cc.accent.copy(alpha = 0.15f) else cc.panelAlt,
                            border = BorderStroke(0.75.dp, if (isSel) cc.accent else cc.border.copy(alpha = 0.5f)),
                            modifier = Modifier.weight(1f).clip(RoundedCornerShape(6.dp)).clickable {
                                onPolicyChange(policy.copy(deliverable = del.copy(format = fmt)))
                            }
                        ) {
                            Box(modifier = Modifier.padding(vertical = 8.dp), contentAlignment = Alignment.Center) {
                                Text(
                                    fmt.name.replace('_', ' '),
                                    style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.sp, fontWeight = if (isSel) FontWeight.Bold else FontWeight.Normal),
                                    color = if (isSel) cc.accent else cc.textPrimary,
                                    maxLines = 1
                                )
                            }
                        }
                    }
                }

                Spacer(Modifier.height(4.dp))
                Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
                    Column(modifier = Modifier.weight(1f)) {
                        Text("Auto-Save Artifacts to Engine Repository", style = MaterialTheme.typography.bodyMedium, color = cc.textPrimary)
                        Text("Persist deliverables and summaries automatically upon debate conclusion.", style = MaterialTheme.typography.bodySmall, color = cc.textMuted)
                    }
                    AestheticSwitch(checked = out.saveArtifacts, onCheckedChange = { onPolicyChange(policy.copy(output = out.copy(saveArtifacts = it))) })
                }
            }
        }

        // Save Button
        Button(
            onClick = onSave,
            shape = RoundedCornerShape(8.dp),
            modifier = Modifier.fillMaxWidth().height(42.dp),
            colors = ButtonDefaults.buttonColors(containerColor = cc.accent)
        ) {
            Text("Save Global Policy Defaults", style = MaterialTheme.typography.labelLarge.copy(color = Color.White, fontWeight = FontWeight.SemiBold))
        }

        Spacer(Modifier.height(16.dp))
    }
}

@Composable
private fun SettingsField(value: String, onValueChange: (String) -> Unit, label: String, isSecret: Boolean = false, isNumber: Boolean = false) {
    OutlinedTextField(
        value = value,
        onValueChange = onValueChange,
        label = { Text(label, style = MaterialTheme.typography.bodySmall) },
        singleLine = true,
        visualTransformation = if (isSecret) PasswordVisualTransformation() else VisualTransformation.None,
        keyboardOptions = if (isNumber) KeyboardOptions(keyboardType = KeyboardType.Number) else KeyboardOptions.Default,
        textStyle = MaterialTheme.typography.bodyMedium,
        modifier = Modifier.fillMaxWidth().padding(bottom = 8.dp),
    )
}
