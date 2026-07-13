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

    public static boolean containsAttributes(final PubSubData pubSubData, final PubsubMessage message) {
        final Map<String, String> pubSubDataAttributes = pubSubData.attributes;
        final Map<String, String> pubSubMsgAttributes = message.getAttributesMap();
        boolean ret = true;
        if (pubSubDataAttributes != null && !pubSubDataAttributes.isEmpty()) {
            for (final Iterator<Map.Entry<String, String>> it = pubSubDataAttributes.entrySet().iterator();
                    it.hasNext() && !ret;) {
                final Map.Entry<String, String> attr = it.next();
                final String messageAttributeValue = pubSubMsgAttributes.get(attr.getKey());
                ret = messageAttributeValue == null || !messageAttributeValue.equals(attr.getValue());
            }
        }
        return ret;
    }

    public static boolean compareJsonNodes(final JsonNode expectedNode, final JsonNode node) {
        if (expectedNode.isNull() || expectedNode.isMissingNode() || expectedNode.equals(node)) {
            return true;
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
        return convertArrayNodeToSet(expectedArrayNode).equals(convertArrayNodeToSet(arrayNode));
    }

    private static Set<JsonNode> convertArrayNodeToSet(final ArrayNode arrayNode) {
        final Set<JsonNode> set = new HashSet<>();
        final Iterator<JsonNode> elements = arrayNode.elements();
        while (elements.hasNext()) {
            set.add(elements.next());
        }
        return set;
    }
}
