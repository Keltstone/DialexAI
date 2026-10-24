package com.dialex.presentation.settings

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.dialex.domain.repository.PersonaRepository
import com.dialex.domain.repository.SettingsRepository
import com.dialex.presentation.nav.AppBackStack
import com.dialex.presentation.nav.PersonaBuilder
import com.dialex.theme.ThemeMode

@Composable
fun SettingsRoute(
    backStack: AppBackStack,
    settingsRepository: SettingsRepository,
    personaRepository: PersonaRepository,
    themeMode: ThemeMode,
    onThemeModeChange: (ThemeMode) -> Unit,
    connectionLabel: String,
    supportsLocalEngine: Boolean,
    onSwitchConnection: () -> Unit,
    recheckCli: suspend () -> List<ProviderStatus>,
    initialTab: SettingsTab = SettingsTab.Appearance,
    profileRepository: com.dialex.domain.repository.ProfileRepository? = null,
    apiKeyRepository: com.dialex.domain.repository.ApiKeyRepository? = null,
    legalConsentRepository: com.dialex.domain.repository.LegalConsentRepository? = null,
    extraTabLabel: String? = null,
    extraTabContent: (@Composable () -> Unit)? = null,
    onExportJson: (String) -> Unit = {},
    isCompact: Boolean = false,
    modifier: Modifier = Modifier
) {
    val viewModel = viewModel {
        SettingsViewModel(
            settingsRepository,
            personaRepository,
            themeMode,
            connectionLabel,
            supportsLocalEngine,
            recheckCli,
            initialTab,
            profileRepository,
            apiKeyRepository,
            legalConsentRepository
        )
    }

    val state by viewModel.state.collectAsStateWithLifecycle()

    LaunchedEffect(initialTab) {
        viewModel.onIntent(SettingsIntent.SelectTab(initialTab))
    }

    LaunchedEffect(viewModel.effect) {
        viewModel.effect.collect { effect ->
            when (effect) {
                is SettingsEffect.SwitchConnection -> {
                    onSwitchConnection()
                    backStack.removeLast()
                }
                is SettingsEffect.NavigateToPersonaBuilder -> backStack.add(PersonaBuilder(effect.personaId))
                is SettingsEffect.ShowSnackbar -> { /* Handle by UI */ }
                is SettingsEffect.ExportPersonasJson -> onExportJson(effect.json)
            }
        }
    }

    LaunchedEffect(state.themeMode) {
        if (state.themeMode != themeMode) {
            onThemeModeChange(state.themeMode)
        }
    }

    SettingsScreen(
        state = state,
        onIntent = viewModel::onIntent,
        onBack = { backStack.removeLast() },
        extraTabLabel = extraTabLabel,
        extraTabContent = extraTabContent,
        onOpenLicensingDedication = { backStack.add(com.dialex.presentation.nav.LicensingDedication) },
        isCompact = isCompact,
        modifier = modifier
    )
}
