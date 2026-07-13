package launcher.postgres;

import java.sql.Connection;
import java.sql.DriverManager;
import java.util.Map;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

public class DatabaseConnectionSingleton {

    private static volatile DatabaseConnectionSingleton instance;

    public volatile Connection connection;

    private static final Logger logger = LoggerFactory.getLogger(DatabaseConnectionSingleton.class);

    private DatabaseConnectionSingleton(final Map<String, Object> config) {
        logger.info("Creating Database connection...");
        final String url = (String) config.get("url");
        final String user = (String) config.get("user");
        final String password = (String) config.get("password");
        try {
            connection = DriverManager.getConnection(url, user, password);
            connection.setAutoCommit(true);
        } catch (Exception e) {
            logger.error("Error connecting to database", e);
        }
    }

    public static synchronized DatabaseConnectionSingleton getInstance(final Map<String, Object> config) {
        if (instance == null) {
            instance = new DatabaseConnectionSingleton(config);
        }
        return instance;
    }
}
