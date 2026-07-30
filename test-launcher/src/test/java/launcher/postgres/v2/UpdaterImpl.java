package launcher.postgres.v2;

import java.util.List;
import java.util.Map;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import launcher.postgres.Updater;

public class UpdaterImpl extends DBConnector implements Updater {

    private final Logger logger = LoggerFactory.getLogger(UpdaterImpl.class);

    public UpdaterImpl(final Map<String, Object> config) {
        super(config);
    }

    @Override
    public Boolean updateField(final String table, final String fieldToUpdate, final Object newValue,
            final Map<String, Object> filter) {
        final StringBuilder sb = new StringBuilder("UPDATE ").append(table)
                .append(" SET ").append(fieldToUpdate).append(" = ? ");
        final List<Object> values = Utils.parametrizeQuery(sb, filter);
        // newValue binds to the first placeholder (the SET clause)
        values.add(0, newValue);
        return executeParamUpdate(sb.toString(), values) > 0;
    }

    @Override
    public Boolean deleteFromTable(final String table, final Map<String, Object> filter) {
        if (filter.isEmpty()) {
            logger.warn("{deleteFromTable} called without any filter value: not allowed");
            return false;
        }
        final StringBuilder sb = new StringBuilder("DELETE FROM ").append(table).append(" ");
        final List<Object> values = Utils.parametrizeQuery(sb, filter);
        return executeParamUpdate(sb.toString(), values) > 0;
    }
}
