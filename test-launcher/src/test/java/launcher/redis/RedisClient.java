package launcher.redis;

import java.util.Map;

import redis.clients.jedis.Jedis;

/**
 * Redis helper for seeding and asserting state against a running Redis.
 * Exposed to Karate via Java.type. Each call uses a short-lived connection.
 */
public class RedisClient {

    private final String host;
    private final int port;
    private final String password;

    /**
     * @param config {@code host} (default localhost), {@code port} (default 6379), optional {@code password}.
     */
    public RedisClient(final Map<String, Object> config) {
        this.host = config.getOrDefault("host", "localhost").toString();
        this.port = Integer.parseInt(config.getOrDefault("port", "6379").toString());
        final Object pwd = config.get("password");
        this.password = pwd != null ? pwd.toString() : null;
    }

    private Jedis open() {
        final Jedis jedis = new Jedis(host, port);
        if (password != null && !password.isEmpty()) {
            jedis.auth(password);
        }
        return jedis;
    }

    /** Set a string value. */
    public String set(final String key, final String value) {
        try (Jedis jedis = open()) {
            return jedis.set(key, value);
        }
    }

    /** Get a string value (null if missing). */
    public String get(final String key) {
        try (Jedis jedis = open()) {
            return jedis.get(key);
        }
    }

    /** Whether a key exists. */
    public Boolean exists(final String key) {
        try (Jedis jedis = open()) {
            return jedis.exists(key);
        }
    }

    /** Delete a key, returning the number of keys removed. */
    public Long del(final String key) {
        try (Jedis jedis = open()) {
            return jedis.del(key);
        }
    }

    /** Set a key with a time-to-live in seconds. */
    public String setEx(final String key, final long seconds, final String value) {
        try (Jedis jedis = open()) {
            return jedis.setex(key, seconds, value);
        }
    }
}
