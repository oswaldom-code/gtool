package launcher.postgres;

import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.SQLException;
import java.util.HashMap;
import java.util.Map;

/**
 * One JDBC connection per thread and per database (url + user). A connection
 * that is missing, closed or no longer valid is reopened, and a database that
 * cannot be reached is an error instead of a silently cached null.
 */
public final class PostgresConnections {

    private static final int VALIDATION_TIMEOUT_SECONDS = 2;
    private static final ThreadLocal<Map<String, Connection>> CONNECTIONS = ThreadLocal.withInitial(HashMap::new);

    private PostgresConnections() {
    }

    public static Connection get(final Map<String, Object> config) throws SQLException {
        final String url = (String) config.get("url");
        final String user = (String) config.get("user");
        final String key = url + "|" + user;
        final Map<String, Connection> connections = CONNECTIONS.get();
        Connection connection = connections.get(key);
        if (connection == null || connection.isClosed() || !connection.isValid(VALIDATION_TIMEOUT_SECONDS)) {
            connection = DriverManager.getConnection(url, user, (String) config.get("password"));
            connection.setAutoCommit(true);
            connections.put(key, connection);
        }
        return connection;
    }
}
