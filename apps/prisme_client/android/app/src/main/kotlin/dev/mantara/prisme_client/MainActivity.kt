package dev.mantara.prisme_client

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.hardware.biometrics.BiometricPrompt
import android.hardware.biometrics.BiometricManager.Authenticators.BIOMETRIC_STRONG
import android.os.Build
import android.os.CancellationSignal
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyPermanentlyInvalidatedException
import android.security.keystore.KeyProperties
import android.util.Base64
import android.view.WindowManager
import com.google.firebase.FirebaseApp
import com.google.firebase.messaging.FirebaseMessaging
import io.flutter.embedding.android.FlutterFragmentActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.Signature
import java.security.spec.ECGenParameterSpec

class MainActivity : FlutterFragmentActivity() {
    private var channel: MethodChannel? = null
    private var pendingRoute: String? = null
    private var permissionResult: MethodChannel.Result? = null
    private var signing: CancellationSignal? = null
    override fun configureFlutterEngine(engine: FlutterEngine) {
        super.configureFlutterEngine(engine)
        window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        captureRoute(intent)
        channel = MethodChannel(engine.dartExecutor.binaryMessenger, "prisme/mobile")
        channel!!.setMethodCallHandler { call, result ->
            try {
                when (call.method) {
                    "initialRoute" -> { result.success(pendingRoute); pendingRoute = null }
                    "notificationStatus" -> result.success(mapOf("configured" to FirebaseApp.getApps(this).isNotEmpty(), "permission" to getSystemService(android.app.NotificationManager::class.java).areNotificationsEnabled()))
                    "enableNotifications" -> {
                        if (FirebaseApp.getApps(this).isEmpty()) result.error("firebase_unconfigured", "Firebase is not configured", null)
                        else if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
                            if (permissionResult != null) result.error("busy", "Permission request active", null)
                            else { permissionResult = result; requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 505) }
                        } else token(result)
                    }
                    "disableNotifications" -> {
                        getSharedPreferences("prisme_push", MODE_PRIVATE).edit().putBoolean("enabled", false).apply()
                        getSystemService(android.app.NotificationManager::class.java).cancelAll()
                        if (FirebaseApp.getApps(this).isNotEmpty()) { FirebaseMessaging.getInstance().isAutoInitEnabled = false; FirebaseMessaging.getInstance().deleteToken() }
                        result.success(null)
                    }
                    "actionPublicKey" -> {
                        if (signing != null) { result.error("busy", "Authentication active", null); return@setMethodCallHandler }
                        val alias = actionAlias(call.argument<String>("scope"))
                        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
                        if (store.containsAlias(alias)) {
                            try {
                                // A certificate can outlive the usable private key after biometric enrollment changes.
                                Signature.getInstance("SHA256withECDSA").initSign(store.getKey(alias, null) as java.security.PrivateKey)
                            } catch (_: KeyPermanentlyInvalidatedException) {
                                // Only explicit enrollment replaces an invalid key; signing never rotates keys.
                                store.deleteEntry(alias)
                            }
                        }
                        if (!store.containsAlias(alias)) {
                            val generator = KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC, "AndroidKeyStore")
                            generator.initialize(KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_SIGN)
                                .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
                                .setDigests(KeyProperties.DIGEST_SHA256).setUserAuthenticationRequired(true)
                                .setUserAuthenticationParameters(0, KeyProperties.AUTH_BIOMETRIC_STRONG)
                                .setInvalidatedByBiometricEnrollment(true).build())
                            generator.generateKeyPair()
                        }
                        result.success(Base64.encodeToString(store.getCertificate(alias).publicKey.encoded, Base64.NO_WRAP))
                    }
                    "signAction" -> {
                        val alias = actionAlias(call.argument<String>("scope"))
                        val challenge = call.argument<String>("challenge") ?: ""
                        if (!challenge.startsWith("prisme-action-v1\n") || challenge.length > 2048) throw IllegalArgumentException()
                        if (signing != null) { result.error("busy", "Authentication active", null); return@setMethodCallHandler }
                        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
                        val signature = Signature.getInstance("SHA256withECDSA").apply { initSign(store.getKey(alias, null) as java.security.PrivateKey) }
                        signing = CancellationSignal()
                        var completed = false
                        fun fail() { if (!completed) { completed = true; signing = null; result.error("biometric_unavailable", "Authentication unavailable or cancelled", null) } }
                        val prompt = BiometricPrompt.Builder(this).setTitle(call.argument<String>("title") ?: "Prisme")
                            .setSubtitle(call.argument<String>("target") ?: "Prisme action").setAllowedAuthenticators(BIOMETRIC_STRONG)
                            .setNegativeButton(getString(android.R.string.cancel), mainExecutor) { _, _ -> fail() }.build()
                        prompt.authenticate(BiometricPrompt.CryptoObject(signature), signing!!, mainExecutor, object : BiometricPrompt.AuthenticationCallback() {
                            override fun onAuthenticationSucceeded(auth: BiometricPrompt.AuthenticationResult) {
                                if (completed) return
                                try { val sig = auth.cryptoObject!!.signature!!; sig.update(challenge.toByteArray(Charsets.UTF_8)); val signed = sig.sign(); completed = true; signing = null; result.success(Base64.encodeToString(signed, Base64.NO_WRAP)) }
                                catch (_: Exception) { fail() }
                            }
                            override fun onAuthenticationError(code: Int, message: CharSequence) { fail() }
                        })
                    }
                    else -> result.notImplemented()
                }
            } catch (_: KeyPermanentlyInvalidatedException) {
                result.error("biometric_key_invalidated", "Sign in with a passkey and re-enroll the biometric key in Notifications", null)
            } catch (_: Exception) { result.error("mobile_unavailable", "Mobile capability unavailable", null) }
        }
    }
    private fun actionAlias(scope: String?): String { require(scope != null && scope.matches(Regex("[a-f0-9]{64}"))); return "prisme-action-$scope" }
    private fun token(result: MethodChannel.Result) {
        if (!getSystemService(android.app.NotificationManager::class.java).areNotificationsEnabled()) { result.error("notification_permission_denied", "Permission denied", null); return }
        FirebaseMessaging.getInstance().isAutoInitEnabled = true
        FirebaseMessaging.getInstance().token.addOnSuccessListener {
            getSharedPreferences("prisme_push", MODE_PRIVATE).edit().putBoolean("enabled", true).apply(); result.success(it)
        }.addOnFailureListener { result.error("push_unavailable", "Push token unavailable", null) }
    }
    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode == 505) { val result = permissionResult; permissionResult = null; if (result != null) token(result) }
    }
    override fun onNewIntent(intent: Intent) { super.onNewIntent(intent); setIntent(intent); captureRoute(intent); pendingRoute?.let { channel?.invokeMethod("route", it); pendingRoute = null } }
    private fun captureRoute(intent: Intent?) {
        val uri = intent?.data
        val route = intent?.getStringExtra("prisme_route") ?: if (uri?.scheme == "prisme" && uri.host == "incidents") "/incidents${uri.path}" else null
        if (route == "/watchdog" || route?.matches(Regex("/incidents/[a-f0-9]{64}")) == true) pendingRoute = route
    }
    override fun onStop() { signing?.cancel(); super.onStop() }
}
