package app.geminicheck

import android.Manifest
import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Color
import android.os.Build
import android.os.Bundle
import android.webkit.JavascriptInterface
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.Toast
import java.net.HttpURLConnection
import java.net.URL

/** A full-screen WebView on the local engine's UI. Android cannot run the signed-in region check, so the page uses connectivity mode. */
class MainActivity : Activity() {
    private lateinit var web: WebView
    private var pendingText: String? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.statusBarColor = Color.BLACK
        window.navigationBarColor = Color.BLACK

        if (Build.VERSION.SDK_INT >= 33 &&
            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 1)
        }
        startForegroundService(Intent(this, ServerService::class.java))

        web = WebView(this)
        web.setBackgroundColor(Color.BLACK)
        web.settings.apply {
            javaScriptEnabled = true
            domStorageEnabled = true // the UI remembers its settings in localStorage
            setSupportZoom(false)
            mixedContentMode = WebSettings.MIXED_CONTENT_NEVER_ALLOW
        }
        web.addJavascriptInterface(Bridge(), "AndroidBridge")
        web.webViewClient = WebViewClient()
        setContentView(web)
        loadWhenReady()
    }

    /** The engine needs a moment to start; poll it off the main thread, then load the UI. */
    private fun loadWhenReady() {
        val base = "http://127.0.0.1:${ServerService.PORT}"
        Thread {
            var up = false
            for (i in 1..80) {
                try {
                    val c = URL("$base/api/version").openConnection() as HttpURLConnection
                    c.connectTimeout = 300
                    c.readTimeout = 300
                    up = c.responseCode == 200
                    c.disconnect()
                } catch (_: Exception) {
                }
                if (up) break
                Thread.sleep(250)
            }
            runOnUiThread {
                if (up) {
                    web.loadUrl("$base/")
                } else {
                    web.loadData(
                        "<body style='background:#000;color:#fff;font-family:sans-serif;padding:24px'>" +
                            "<h3>The engine did not start</h3><p>Close the app and open it again.</p></body>",
                        "text/html",
                        "utf-8",
                    )
                }
            }
        }.start()
    }

    /** The page calls this to save exports (blob downloads do not work inside a WebView). */
    inner class Bridge {
        @JavascriptInterface
        fun saveText(name: String, text: String) {
            pendingText = text
            runOnUiThread {
                val intent = Intent(Intent.ACTION_CREATE_DOCUMENT)
                    .addCategory(Intent.CATEGORY_OPENABLE)
                    .setType("text/plain")
                    .putExtra(Intent.EXTRA_TITLE, name)
                @Suppress("DEPRECATION")
                startActivityForResult(intent, REQUEST_SAVE)
            }
        }
    }

    @Suppress("DEPRECATION")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != REQUEST_SAVE) return
        val uri = data?.data
        val text = pendingText
        pendingText = null
        if (resultCode == RESULT_OK && uri != null && text != null) {
            contentResolver.openOutputStream(uri)?.use { it.write(text.toByteArray()) }
            Toast.makeText(this, R.string.saved, Toast.LENGTH_SHORT).show()
        }
    }

    @Suppress("DEPRECATION")
    override fun onBackPressed() {
        if (web.canGoBack()) web.goBack() else super.onBackPressed()
    }

    companion object {
        private const val REQUEST_SAVE = 7
    }
}
