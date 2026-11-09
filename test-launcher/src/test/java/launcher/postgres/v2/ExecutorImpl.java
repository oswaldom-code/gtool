package launcher.postgres.v2;

import java.sql.Connection;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.Map;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import launcher.postgres.Executor;
import launcher.postgres.PostgresConnections;
import launcher.util.SqlScript;

public class ExecutorImpl extends DBConnector implements Executor {

    private final Logger logger = LoggerFactory.getLogger(ExecutorImpl.class);

    public ExecutorImpl(final Map<String, Object> config) {
        super(config);
    }

    /** Runs the script in one transaction: it applies completely or not at all. */
    @Override
    public Boolean executeSqlScript(final String scriptName, final String sql) throws SQLException {
        final Connection con = PostgresConnections.get(config);
        final String name = scriptName != null ? scriptName : "";
        String query = "";
        con.setAutoCommit(false);
        try (Statement st = con.createStatement()) {
            for (final String statement : SqlScript.split(sql, true)) {
                query = statement;
                st.execute(statement);
            }
            con.commit();
            return true;
        } catch (SQLException e) {
            logger.error("{executeSqlScript} Could not execute script {}. Error executing query {}.", name, query, e);
            con.rollback();
            return false;
        } finally {
            con.setAutoCommit(true);
        }
    }
}
