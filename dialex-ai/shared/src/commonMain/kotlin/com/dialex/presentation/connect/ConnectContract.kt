package com.dialex.presentation.connect

import kolt.utils.state.AsyncState
// Connect Screen Contract — State / Intent / Effect
data class ConnectState(
    val supportsLocalEngine: Boolean = false,
    val remoteUrl: String = "",
    val remoteUsername: String = "",
    val remotePassword: String = "",
    val connectAsync: AsyncState<Unit> = AsyncState.Idle,
    val startingLocalEngine: Boolean = false,
    /** Error message shown below the form; null when no error. */
    val error: String? = null,
)

sealed interface ConnectIntent {
    data class UrlChanged(val url: String) : ConnectIntent
    data class UsernameChanged(val username: String) : ConnectIntent
    data class PasswordChanged(val password: String) : ConnectIntent
    data object ConnectToRemote : ConnectIntent
    data object UseThisDevice : ConnectIntent
    data object Reset : ConnectIntent
    data object DismissError : ConnectIntent
}

sealed interface ConnectEffect {
    /** Navigation — engine is connected, proceed to the main app. */
    data object NavigateToMain : ConnectEffect
}
