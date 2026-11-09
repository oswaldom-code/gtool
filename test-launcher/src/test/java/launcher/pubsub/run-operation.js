function(operation, args, debug) {
  var PubSub = Java.type('launcher.pubsub.PubSub');
  var PubSubData = Java.type('launcher.pubsub.PubSub$PubSubData');
  var Operation = Java.type('launcher.pubsub.operations.' + operation);
  var fields = ['projectId', 'topicId', 'subscriptionId', 'message', 'expectedMessage', 'ceType', 'ceSource',
    'ignoreMessageFields', 'orderedMessages', 'orderingKey', 'maxRequestedMessages',
    'timeoutConsumeMessageSeconds', 'attributes'];
  var data = new PubSubData();
  for (var i = 0; i < fields.length; i++) {
    var value = args[fields[i]];
    if (value !== undefined && value !== null) data[fields[i]] = value;
  }
  var pubsub = new PubSub();
  return debug ? pubsub.runOperationOnEmulatorDebug(new Operation(), data)
               : pubsub.runOperationOnEmulator(new Operation(), data);
}
