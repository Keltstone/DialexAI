package com.dialex.presentation.legal.licensing

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.flowWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.dialex.presentation.nav.AppBackStack

/**
 * Route composable governing the Licensing Dedication screen.
 * Connects the LicensingViewModel and backStack navigation effects to the stateless screen.
 */
@Composable
fun LicensingRoute(
    backStack: AppBackStack,
    onBack: () -> Unit,
    modifier: Modifier = Modifier
) {
    val viewModel: LicensingViewModel = viewModel { LicensingViewModel() }
    val state by viewModel.state.collectAsStateWithLifecycle()
    val lifecycle = LocalLifecycleOwner.current.lifecycle

    LaunchedEffect(viewModel, lifecycle) {
        viewModel.effect.flowWithLifecycle(lifecycle).collect { effect ->
            when (effect) {
                is LicensingEffect.NavigateBack -> {
                    onBack()
                }
                is LicensingEffect.ShowSnackbar -> {
                    // Handled internally in screen banner
                }
            }
        }
    }

    LicensingScreen(
        state = state,
        onIntent = viewModel::onIntent,
        onBack = onBack,
        modifier = modifier
    )
}
