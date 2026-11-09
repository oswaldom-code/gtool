package launcher.util;

import java.util.Locale;
import java.util.Map;

/**
 * Resolves the host ports of the current gtool run. gtool injects
 * GTOOL_&lt;SERVICE&gt;_PORT (and GTOOL_APP_URL) into the launcher; without them
 * the plugin defaults apply.
 * Exposed to Karate via Java.type.
 */
public final class RunPorts {

    private static final String DEFAULT_APP_URL = "http://localhost:8080";

    private RunPorts() {
    }

    /** Env variable gtool uses for a service port, e.g. kafka -&gt; GTOOL_KAFKA_PORT. */
    static String envName(final String service) {
        return "GTOOL_" + service.toUpperCase(Locale.ROOT).replace('-', '_') + "_PORT";
    }

    static String resolve(final Map<String, String> env, final String key, final String fallback) {
        final String value = env.get(key);
        return value == null || value.isBlank() ? fallback : value.trim();
    }

    /** Host port of a mock service for this run, or fallback when gtool did not set it. */
    public static String port(final String service, final String fallback) {
        return resolve(System.getenv(), envName(service), fallback);
    }

    /** A mock connection setting (user, password, database) for this run, e.g. GTOOL_POSTGRESQL_USER. */
    public static String setting(final String service, final String key, final String fallback) {
        final String name = "GTOOL_" + service.toUpperCase(Locale.ROOT).replace('-', '_') + "_"
                + key.toUpperCase(Locale.ROOT).replace('-', '_');
        return resolve(System.getenv(), name, fallback);
    }

    /** Base URL of the app under test for this run. */
    public static String appUrl() {
        return resolve(System.getenv(), "GTOOL_APP_URL", DEFAULT_APP_URL);
    }
}
