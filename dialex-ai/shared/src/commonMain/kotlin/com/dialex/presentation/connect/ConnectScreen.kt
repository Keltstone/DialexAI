package com.dialex.presentation.connect

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.RadioButtonChecked
import androidx.compose.material.icons.outlined.RadioButtonUnchecked
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import com.dialex.theme.LocalCcColors
import com.dialex.ui.AestheticRadioButton
import com.dialex.ui.DialexLogoView
import org.jetbrains.compose.ui.tooling.preview.Preview

@Composable
fun ConnectScreen(
    state: ConnectState,
    onIntent: (ConnectIntent) -> Unit,
    modifier: Modifier = Modifier
) {
    val cc = LocalCcColors.current
    var remoteMode by remember { mutableStateOf(!state.supportsLocalEngine) }

    Box(modifier.fillMaxSize().background(cc.bg), Alignment.Center) {
        Column(
            Modifier.width(360.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            DialexLogoView(size = 56.dp, shape = RoundedCornerShape(12.dp))
            Spacer(Modifier.height(12.dp))
            Text("Dialex", style = MaterialTheme.typography.headlineSmall, color = cc.textPrimary)
            Text("Keltstone Systems", style = MaterialTheme.typography.bodySmall, color = cc.textMuted)
            Spacer(Modifier.height(24.dp))

            if (state.supportsLocalEngine) {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(12.dp))
                        .background(cc.panelAlt)
                        .border(1.dp, cc.border, RoundedCornerShape(12.dp))
                        .padding(4.dp),
                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                ) {
                    // Option 1: Local Engine
                    Box(
                        modifier = Modifier
                            .weight(1f)
                            .clip(RoundedCornerShape(8.dp))
                            .background(if (!remoteMode) cc.panelAlt else Color.Transparent)
                            .border(
                                width = if (!remoteMode) 1.dp else 0.dp,
                                color = if (!remoteMode) cc.border else Color.Transparent,
                                shape = RoundedCornerShape(8.dp)
                            )
                            .clickable { remoteMode = false }
                            .padding(vertical = 10.dp, horizontal = 10.dp),
                        contentAlignment = Alignment.Center
                    ) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            AestheticRadioButton(
                                selected = !remoteMode,
                                onClick = null,
                                size = 18.dp
                            )
                            Spacer(Modifier.width(7.dp))
                            Text(
                                "This device",
                                style = MaterialTheme.typography.bodyMedium.copy(
                                    fontWeight = if (!remoteMode) FontWeight.SemiBold else FontWeight.Normal
                                ),
                                color = cc.textPrimary
                            )
                        }
                    }

                    // Option 2: Another Engine (Remote)
                    Box(
                        modifier = Modifier
                            .weight(1f)
                            .clip(RoundedCornerShape(8.dp))
                            .background(if (remoteMode) cc.panelAlt else Color.Transparent)
                            .border(
                                width = if (remoteMode) 1.dp else 0.dp,
                                color = if (remoteMode) cc.border else Color.Transparent,
                                shape = RoundedCornerShape(8.dp)
                            )
                            .clickable { remoteMode = true }
                            .padding(vertical = 10.dp, horizontal = 10.dp),
                        contentAlignment = Alignment.Center
                    ) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            AestheticRadioButton(
                                selected = remoteMode,
                                onClick = null,
                                size = 18.dp
                            )
                            Spacer(Modifier.width(7.dp))
                            Text(
                                "Another engine",
                                style = MaterialTheme.typography.bodyMedium.copy(
                                    fontWeight = if (remoteMode) FontWeight.SemiBold else FontWeight.Normal
                                ),
                                color = cc.textPrimary
                            )
                        }
                    }
                }
                Spacer(Modifier.height(18.dp))
            }

            if (!remoteMode) {
                Text(
                    "Runs the engine on this device — nothing to configure, works offline.",
                    style = MaterialTheme.typography.bodySmall,
                    color = cc.textMuted,
                )
                Spacer(Modifier.height(16.dp))
                com.dialex.ui.GradientButton(
                    text = if (state.startingLocalEngine) "Starting…" else "Use this device",
                    onClick = { onIntent(ConnectIntent.UseThisDevice) },
                    enabled = !state.startingLocalEngine,
                    height = 44.dp,
                    modifier = Modifier.fillMaxWidth()
                )
            } else {
                Text(
                    "Connect to an engine already running elsewhere — a desktop, or any " +
                        "other self-hosted Dialex engine on your network.",
                    style = MaterialTheme.typography.bodySmall,
                    color = cc.textMuted,
                )
                Spacer(Modifier.height(16.dp))
                OutlinedTextField(
                    value = state.remoteUrl,
                    onValueChange = { onIntent(ConnectIntent.UrlChanged(it)) },
                    label = { Text("Engine URL, e.g. http://192.168.1.20:7890") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                Spacer(Modifier.height(8.dp))
                OutlinedTextField(
                    value = state.remoteUsername,
                    onValueChange = { onIntent(ConnectIntent.UsernameChanged(it)) },
                    label = { Text("Username") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                Spacer(Modifier.height(8.dp))
                OutlinedTextField(
                    value = state.remotePassword,
                    onValueChange = { onIntent(ConnectIntent.PasswordChanged(it)) },
                    label = { Text("Password") },
                    singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    modifier = Modifier.fillMaxWidth(),
                )
                Spacer(Modifier.height(16.dp))
                com.dialex.ui.GradientButton(
                    text = if (state.connectAsync.isLoading) "Connecting…" else "Connect",
                    onClick = { onIntent(ConnectIntent.ConnectToRemote) },
                    enabled = !state.connectAsync.isLoading && state.remoteUrl.isNotBlank() && state.remoteUsername.isNotBlank() && state.remotePassword.isNotBlank(),
                    height = 44.dp,
                    modifier = Modifier.fillMaxWidth()
                )
            }

            if (state.error != null) {
                Spacer(Modifier.height(12.dp))
                Text(state.error, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
                TextButton(onClick = { onIntent(ConnectIntent.DismissError) }) {
                    Text("Dismiss")
                }
            }
        }
    }
}

@Preview
@Composable
fun ConnectScreenPreview() {
    ConnectScreen(
        state = ConnectState(supportsLocalEngine = true),
        onIntent = {}
    )
}
