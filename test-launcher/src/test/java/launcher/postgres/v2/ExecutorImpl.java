package launcher.postgres.v2;

import java.sql.Connection;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.Map;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import launcher.postgres.DatabaseConnectionSingleton;
import launcher.postgres.Executor;

public class ExecutorImpl extends DBConnector implements Executor {

    private final Logger logger = LoggerFactory.getLogger(ExecutorImpl.class);

    public ExecutorImpl(final Map<String, Object> config) {
        super(config);
    }

    @Override
    public Boolean executeSqlScript(final String scriptName, final String sql) throws SQLException {
        final Connection con = DatabaseConnectionSingleton.getInstance(config).connection;
        final String[] statements = sql.split(";");
        boolean ok = true;
        String query = "";

        try (Statement st = con.createStatement()) {
            for (final String statement : statements) {
                query = statement + ";";
                if (!query.trim().equals(";")) {
                    st.executeUpdate(query);
                }
                query = "";
            }
        } catch (SQLException e) {
            final String name = scriptName != null ? scriptName : "";
            logger.error("{executeSqlScript} Could not execute script {}. Error executing query {}.", name, query, e);
            try {
                con.rollback();
            } catch (SQLException re) {
                logger.warn("{executeSqlScript} could not rollback after error.", re);
            }
            ok = false;
        }
        return ok;
    }
}
