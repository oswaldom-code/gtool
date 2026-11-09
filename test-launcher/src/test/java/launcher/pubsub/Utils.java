package launcher.pubsub;

import java.util.HashSet;
import java.util.Iterator;
import java.util.Map;
import java.util.Set;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.google.api.client.util.Strings;
import com.google.pubsub.v1.PubsubMessage;
import com.google.pubsub.v1.ReceivedMessage;

import launcher.pubsub.PubSub.PubSubData;

/**
 * Static helpers for Pub/Sub message handling (JSON mapping, ordering and attribute checks).
 */
public final class Utils {

    private Utils() {
    }

    private static final Logger logger = LoggerFactory.getLogger(Utils.class);

    /** Maps a message to a JsonNode, removing {@link PubSubData#ignoreMessageFields}. */
    public static JsonNode mapperMessageToJsonNode(final String message, final PubSubData pubSubData) {
        final Set<String> ignoreMessageFields = new HashSet<>();
        if (pubSubData.ignoreMessageFields != null) {
            ignoreMessageFields.addAll(pubSubData.ignoreMessageFields);
        }
        return mapperMessageToJsonNode(message, ignoreMessageFields);
    }

    /** Maps a message to a JsonNode, removing the given fields. */
    public static JsonNode mapperMessageToJsonNode(final String message, final Set<String> ignoreMessageFields) {
        final ObjectMapper mapper = new ObjectMapper();
        try {
            final JsonNode jsonNode = mapper.readTree(message);
            if (ignoreMessageFields != null && !ignoreMessageFields.isEmpty()) {
                ignoreMessageFields.forEach(field -> deleteField(jsonNode, field));
            }
            return jsonNode;
        } catch (JsonProcessingException jex) {
            throw new RuntimeException("Error parsing message: " + message, jex);
        }
    }

    /** Maps a message to a Map, removing {@link PubSubData#ignoreMessageFields}. */
    @SuppressWarnings("unchecked")
    public static Map<String, Object> mapperMessageToMap(final String message, final PubSubData pubSubData) {
        final ObjectMapper mapper = new ObjectMapper();
        final JsonNode jsonNode = mapperMessageToJsonNode(message, pubSubData);
        try {
            return mapper.readValue(jsonNode.toString(), Map.class);
        } catch (JsonProcessingException e) {
            throw new RuntimeException("Error parsing message: " + message, e);
        }
    }

    /** Removes a field (supports nested {@code parent.child} paths) from a JsonNode. */
    public static void deleteField(final JsonNode node, final String fieldName) {
        if (node.isNull()) {
            return;
        }
        if (node.isArray()) {
            for (final JsonNode item : node) {
                deleteField(item, fieldName);
            }
            return;
        }
        final int position = fieldName.indexOf('.');
        if (position < 0) {
            ((ObjectNode) node).remove(fieldName);
        } else {
            final JsonNode childNode = node.get(fieldName.substring(0, position));
            if (childNode == null) {
                logger.warn("Error removing the field {}", fieldName);
            } else {
                deleteField(childNode, fieldName.substring(position + 1));
            }
        }
    }

    public static boolean isValidOrdering(final PubSubData pubSubData, final ReceivedMessage message) {
        return isValidOrdering(pubSubData, message.getMessage());
    }

    public static boolean isValidOrdering(final PubSubData pubSubData, final PubsubMessage message) {
        boolean valid = true;
        if (!Strings.isNullOrEmpty(pubSubData.orderingKey)) {
            valid = pubSubData.orderingKey.equals(message.getOrderingKey());
        }
        return valid;
    }

    public static boolean containsAttributes(final PubSubData pubSubData, final ReceivedMessage message) {
        return containsAttributes(pubSubData, message.getMessage());
    }

    /** True when the message carries every expected attribute with the same value. */
    public static boolean containsAttributes(final PubSubData pubSubData, final PubsubMessage message) {
        final Map<String, String> expected = pubSubData.attributes;
        if (expected == null || expected.isEmpty()) {
            return true;
        }
        final Map<String, String> actual = message.getAttributesMap();
        for (final Map.Entry<String, String> attr : expected.entrySet()) {
            if (!attr.getValue().equals(actual.get(attr.getKey()))) {
                return false;
            }
        }
        return true;
    }

    /**
     * Lenient comparison: a null expected value matches anything, objects must
     * have the same fields, and arrays match regardless of order (duplicates
     * count).
     */
    public static boolean compareJsonNodes(final JsonNode expectedNode, final JsonNode node) {
        if (expectedNode.isNull() || expectedNode.isMissingNode() || expectedNode.equals(node)) {
            return true;
        }
        if (node == null || node.isMissingNode()) {
            return false;
        }
        if (expectedNode.isArray() && node.isArray()) {
            return compareArrays((ArrayNode) expectedNode, (ArrayNode) node);
        }
        if (expectedNode.isObject() && node.isObject()) {
            if (expectedNode.size() != node.size()) {
                return false;
            }
            final Iterator<String> fieldNames = expectedNode.fieldNames();
            while (fieldNames.hasNext()) {
                final String fieldName = fieldNames.next();
                if (!compareJsonNodes(expectedNode.get(fieldName), node.get(fieldName))) {
                    return false;
                }
            }
            return true;
        }
        return false;
    }

    private static boolean compareArrays(final ArrayNode expectedArrayNode, final ArrayNode arrayNode) {
        if (expectedArrayNode.size() != arrayNode.size()) {
            return false;
        }
        final boolean[] used = new boolean[arrayNode.size()];
        for (final JsonNode expected : expectedArrayNode) {
            if (!claimMatch(expected, arrayNode, used)) {
                return false;
            }
        }
        return true;
    }

    private static boolean claimMatch(final JsonNode expected, final ArrayNode actual, final boolean[] used) {
        for (int i = 0; i < actual.size(); i++) {
            if (!used[i] && compareJsonNodes(expected, actual.get(i))) {
                used[i] = true;
                return true;
            }
        }
        return false;
    }
}
