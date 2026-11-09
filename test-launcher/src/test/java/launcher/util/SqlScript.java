package launcher.util;

import java.util.ArrayList;
import java.util.List;

/**
 * Splits a SQL script into statements on top-level semicolons, ignoring the
 * ones inside quotes, comments and (for PostgreSQL) dollar-quoted bodies such
 * as {@code $$ ... $$} or {@code $fn$ ... $fn$}.
 */
public final class SqlScript {

    private SqlScript() {
    }

    public static List<String> split(final String sql, final boolean dollarQuotes) {
        final List<String> statements = new ArrayList<>();
        final StringBuilder current = new StringBuilder();
        int i = 0;
        while (i < sql.length()) {
            final char c = sql.charAt(i);
            final int end;
            if (c == '\'' || c == '"' || c == '`') {
                end = closing(sql, i + 1, String.valueOf(c));
            } else if (c == '-' && sql.startsWith("--", i)) {
                end = lineEnd(sql, i);
            } else if (c == '/' && sql.startsWith("/*", i)) {
                end = closing(sql, i + 2, "*/");
            } else if (c == '$' && dollarQuotes && dollarTag(sql, i) != null) {
                final String tag = dollarTag(sql, i);
                end = closing(sql, i + tag.length(), tag);
            } else if (c == ';') {
                add(statements, current);
                i++;
                continue;
            } else {
                current.append(c);
                i++;
                continue;
            }
            current.append(sql, i, end);
            i = end;
        }
        add(statements, current);
        return statements;
    }

    private static void add(final List<String> statements, final StringBuilder current) {
        final String statement = current.toString().trim();
        if (!statement.isEmpty() && !isOnlyComments(statement)) {
            statements.add(statement);
        }
        current.setLength(0);
    }

    private static boolean isOnlyComments(final String statement) {
        final StringBuilder code = new StringBuilder();
        int i = 0;
        while (i < statement.length()) {
            if (statement.startsWith("--", i)) {
                i = lineEnd(statement, i);
            } else if (statement.startsWith("/*", i)) {
                i = closing(statement, i + 2, "*/");
            } else {
                code.append(statement.charAt(i++));
            }
        }
        return code.toString().trim().isEmpty();
    }

    /** Index just after the closing token, or the end of the script when unterminated. */
    private static int closing(final String sql, final int from, final String token) {
        final int idx = sql.indexOf(token, from);
        return idx < 0 ? sql.length() : idx + token.length();
    }

    private static int lineEnd(final String sql, final int from) {
        final int idx = sql.indexOf('\n', from);
        return idx < 0 ? sql.length() : idx + 1;
    }

    /** Returns the dollar-quote tag ($$ or $name$) starting at i, or null. */
    private static String dollarTag(final String sql, final int i) {
        int j = i + 1;
        while (j < sql.length() && (Character.isLetterOrDigit(sql.charAt(j)) || sql.charAt(j) == '_')) {
            j++;
        }
        if (j < sql.length() && sql.charAt(j) == '$' && (j == i + 1 || !Character.isDigit(sql.charAt(i + 1)))) {
            return sql.substring(i, j + 1);
        }
        return null;
    }
}
