// Package csxamqp is a wrapper for amqp package
// with reconnect functional support
package csxamqp

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"gitlab.com/battler/modules/csxstrings"
)

var (
	consumers     sync.Map
	consumersLock sync.Mutex

	updateExch  = os.Getenv("EXCHANGE_UPDATES")
	amqpURI     = os.Getenv("AMQP_URI")
	envName     = os.Getenv("CSX_ENV")
	consumerTag = os.Getenv("SERVICE_NAME")
)

// Update struc for send updates msg to services
type Update struct {
	ID         string      `json:"id"`
	ExtID      string      `json:"extId"`
	Cmd        string      `json:"cmd"`
	Collection string      `json:"collection"`
	Data       string      `json:"data"`
	Groups     []string    `json:"groups"`
	ExtData    interface{} `json:"extData"`
	Recipients []string    `json:"recipients"`
	Initiator  string      `json:"initiator,omitempty"`
}

//Consumer structure for NewConsumer result
type Consumer struct {
	conn     *amqp.Connection
	channel  *amqp.Channel
	done     chan error
	exchange *Exchange
	queue    *Queue
	uri      string
	name     string // consumer name for logs
	handlers sync.Map
}

// Exchange struct for receive exchange params
type Exchange struct {
	Name       string
	Type       string
	Durable    bool
	AutoDelete bool
	Internal   bool
	NoWait     bool
	Args       map[string]interface{}
}

// Queue struct for receive queue params
type Queue struct {
	Name        string
	AutoDelete  bool
	Durable     bool
	ConsumerTag string
	Keys        []string
	BindedKeys  []string
	NoAck       bool
	NoLocal     bool
	Exclusive   bool
	NoWait      bool
	Args        map[string]interface{}
}

type Delivery amqp.Delivery
type Table amqp.Table

var (
	defaultAmqpReconnectionTime = 20 // in seconds

	reconTime = prepareReconnectionTime() // in seconds
)

func (c *Consumer) logInfo(log string) string {
	return "[" + c.name + "]" + log
}

func prepareReconnectionTime() time.Duration {
	amqpReconTime, err := strconv.Atoi(os.Getenv("AMQP_RECONNECTION_TIME"))
	if err != nil {
		amqpReconTime = defaultAmqpReconnectionTime
	}
	return time.Second * time.Duration(amqpReconTime)
}

// GetReconnectionTime returns AMQP reconnection time
func GetReconnectionTime() time.Duration {
	return reconTime
}

// ExchangeDeclare dial amqp server, decrare echange an queue if set
func (c *Consumer) ExchangeDeclare() (string, error) {
	if c.exchange != nil {
		logrus.Info(c.logInfo("amqp declaring exchange: "), c.exchange.Name, " type: ", c.exchange.Type, " durable: ", c.exchange.Durable, " autodelete: ", c.exchange.AutoDelete)
		if err := c.channel.ExchangeDeclare(
			c.exchange.Name,       // name of the exchange
			c.exchange.Type,       // type
			c.exchange.Durable,    // durable
			c.exchange.AutoDelete, // delete when complete
			c.exchange.Internal,   // internal
			c.exchange.NoWait,     // noWait
			c.exchange.Args,       // arguments
		); err != nil {
			return "", err
		}
		return c.exchange.Name, nil
	}
	return "", nil
}

// BindKeys dial amqp server, decrare echange an queue if set
func (c *Consumer) BindKeys(keys []string) error {
	if len(keys) > 0 {
		if len(c.queue.Keys) == 0 {
			c.queue.Keys = make([]string, 0)
		}
		for _, key := range keys {
			keyExists := false
			for _, oldKey := range c.queue.Keys {
				if oldKey == key {
					keyExists = true
					break
				}
			}
			if !keyExists {
				err := c.channel.QueueBind(
					c.queue.Name,    // name of the queue
					key,             // bindingKey
					c.exchange.Name, // sourceExchange
					c.queue.NoWait,  // noWait
					c.queue.Args,    // arguments
				)
				if err != nil {
					return err
				}
				c.queue.Keys = append(c.queue.Keys, key)
			}
		}
		logrus.Info(c.logInfo("amqp bind keys: "), keys, " to queue: ", c.queue.Name)
	}
	return nil
}

// QueueDeclare dial amqp server, decrare echange an queue if set
func (c *Consumer) QueueDeclare(exchange string, keys []string) (<-chan amqp.Delivery, error) {
	// declare and bind queue
	if c.queue != nil {
		logrus.Info(c.logInfo("amqp declare queue: "), c.queue.Name, " durable: ", c.queue.Durable, " autodelete: ", c.queue.AutoDelete)
		_, err := c.channel.QueueDeclare(
			c.queue.Name,       // name of the queue
			c.queue.Durable,    // durable
			c.queue.AutoDelete, // delete when unused
			c.queue.Exclusive,  // exclusive
			c.queue.NoWait,     // noWait
			c.queue.Args,       // arguments
		)
		if err != nil {
			return nil, err
		}
		if len(c.queue.Keys) > 0 && keys == nil {
			err = c.BindKeys(c.queue.Keys)
		} else if len(keys) > 0 {
			err = c.BindKeys(keys) // if reconnect or create new consumer cases
		} else {
			err = c.BindKeys([]string{c.queue.ConsumerTag})
		}

		if err != nil {
			return nil, err
		}
		logrus.Info(c.logInfo("starting consume for queue: "), c.queue.Name)
		deliveries, err := c.channel.Consume(
			c.queue.Name,        // name
			c.queue.ConsumerTag, // consumerTag,
			c.queue.NoAck,       // noAck
			c.queue.Exclusive,   // exclusive
			c.queue.NoLocal,     // noLocal
			c.queue.NoWait,      // noWait
			c.queue.Args,        // arguments
		)
		if err != nil {
			return nil, err
		}
		return deliveries, nil

	}
	return nil, nil
}

// Connect dial amqp server, decrare echange an queue if set
func (c *Consumer) Connect(reconnect bool, keys []string) (<-chan amqp.Delivery, error) {
	var err error
	logrus.Info(c.logInfo("amqp connect to: "), c.uri)
	c.conn, err = amqp.Dial(c.uri)
	if err != nil {
		return nil, err
	}

	logrus.Info(c.logInfo("amqp get channel"))
	c.channel, err = c.conn.Channel()
	if err != nil {
		return nil, err
	}
	// declare exchange
	exchange, err := c.ExchangeDeclare()
	if err != nil {
		return nil, err
	}
	// declare queue and bind routing keys
	deliveries, err := c.QueueDeclare(exchange, keys)
	if err != nil {
		return nil, err
	}
	if !reconnect {
		go c.handleDeliveries(deliveries)
	}
	go func() {
		logrus.Error(c.logInfo("amqp connection err: "), <-c.conn.NotifyClose(make(chan *amqp.Error)))
		c.done <- errors.New(c.logInfo("channel closed"))
	}()
	return deliveries, nil
}

// Reconnect reconnect to amqp server
func (c *Consumer) Reconnect(keys []string) <-chan amqp.Delivery {
	if err := c.Shutdown(); err != nil {
		logrus.Error(c.logInfo("error during shutdown: "), err)
	}
	reconnectInterval := 30
	logrus.Warn(c.logInfo("consumer wait reconnect"), " next try in ", reconnectInterval, "s")
	time.Sleep(time.Duration(reconnectInterval) * time.Second)
	deliveries, err := c.Connect(true, keys)
	if err != nil {
		logrus.Error(c.logInfo("consumer reconnect err: "), err.Error(), " next try in ", reconnectInterval, "s")
		return c.Reconnect(keys)
	}
	return deliveries
}

//NewConsumer create simple consumer for read messages with ack
func NewConsumer(amqpURI, name string, exchange *Exchange, queue *Queue, handlers []func(*Delivery)) (*Consumer, error) {
	c := &Consumer{
		exchange: exchange,
		queue:    queue,
		done:     make(chan error),
		uri:      amqpURI,
		name:     name,
	}
	if len(queue.Keys) > 0 && len(handlers) > 0 {
		for i := 0; i < len(handlers); i++ {
			handler := handlers[i]
			c.AddConsumeHandler(queue.Keys, handler)
		}
	} else if len(handlers) > 0 {
		c.handlers = sync.Map{}
		c.handlers.Store("-", handlers)
	}

	var keys []string
	if len(c.queue.Keys) > 0 {
		keys = make([]string, len(c.queue.Keys))
		copy(keys, c.queue.Keys)
		c.queue.Keys = nil
	}
	_, err := c.Connect(false, keys)
	return c, err
}

// NewPublisher create publisher for send amqp messages
func NewPublisher(amqpURI, name string, exchange Exchange) (*Consumer, error) {
	c := &Consumer{
		exchange: &exchange,
		uri:      amqpURI,
		name:     name,
	}
	_, err := c.Connect(false, nil)
	return c, err
}

// NewPublisher create publisher for send amqp messages
func NewPublisherWithRPC(amqpURI, name string, exchange Exchange, rpcQueueName string, keys []string, cb ...func(*Delivery)) (*Consumer, error) {
	c, err := NewPublisher(amqpURI, name, exchange)
	if err != nil {
		return nil, err
	}
	queue := Queue{
		Name:        rpcQueueName,
		ConsumerTag: rpcQueueName,
		AutoDelete:  true,
		Durable:     false,
		Keys:        keys,
	}
	if err != nil {
		return nil, err
	}
	_, err = NewConsumer(amqpURI, name, &exchange, &queue, cb)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// PublishWithHeaders sends messages and reconnects in case of error
func (c *Consumer) PublishWithHeaders(msg []byte, routingKey string, headers map[string]interface{}) error {
	content := amqp.Publishing{
		ContentType: "text/plain",
		Body:        msg,
	}
	if headers != nil {
		content.Headers = headers
	}
	return c.channelPublish(msg, routingKey, &content)
}

func (c *Consumer) channelPublish(msg []byte, routingKey string, content *amqp.Publishing) error {
	if c == nil {
		c.Reconnect(nil)
		logrus.Error(c.logInfo("reconnected after c == nil"))
		return c.PublishWithHeaders(msg, routingKey, content.Headers)
	}
	if c.conn == nil {
		c.Reconnect(nil)
		logrus.Error(c.logInfo("reconnected after c.conn == nil"))
		return c.PublishWithHeaders(msg, routingKey, content.Headers)
	}
	if c.channel == nil {
		c.Reconnect(nil)
		logrus.Error(c.logInfo("reconnected after c.channel == nil"))
		return c.PublishWithHeaders(msg, routingKey, content.Headers)
	}
	errPublish := c.channel.Publish(c.exchange.Name, routingKey, false, false, *content)
	if errPublish != nil {
		c.Reconnect(nil)
		logrus.Error(c.logInfo("reconnected after publish err: "), errPublish)
		return c.PublishWithHeaders(msg, routingKey, content.Headers)
	}
	return nil
}

// Publish sends messages and reconnects in case of error
func (c *Consumer) Publish(msg []byte, routingKey string) error {
	return c.PublishWithHeaders(msg, routingKey, nil)
}

// PublishWithReply sends messages to receiver that passed in replyTo parameter and correlationId of delivery
func (c *Consumer) PublishWithReply(msg []byte, routingKey, replyTo, correlationId string, headers map[string]interface{}) error {
	content := amqp.Publishing{
		ContentType:   "text/plain",
		Body:          msg,
		ReplyTo:       replyTo,
		CorrelationId: correlationId,
		Headers:       headers,
	}
	return c.channelPublish(msg, routingKey, &content)
}

// GetConsumer get or create publish/consume consumer
func GetConsumer(amqpURI, name string, exchange *Exchange, queue *Queue, handler func(*Delivery)) (consumer *Consumer, err error) {
	consumersLock.Lock()
	defer consumersLock.Unlock()
	consumerInt, ok := consumers.Load(name)
	if !ok {
		if queue == nil {
			exch := Exchange{}
			if exchange != nil {
				exch = *exchange
			}
			consumer, err = NewPublisher(amqpURI, name, exch)
		} else {
			consumer, err = NewConsumer(amqpURI, name, exchange, queue, nil)
		}
		if err != nil {
			return nil, err
		}
		consumers.Store(name, consumer)
	} else {
		consumer = consumerInt.(*Consumer)
		var err error
		if queue != nil {
			if queue.Keys != nil && len(queue.Keys) > 0 {
				err = consumer.BindKeys(queue.Keys)
			} else {
				err = consumer.BindKeys([]string{queue.ConsumerTag})
			}
			if err != nil {
				return consumer, err
			}
		}
	}
	return consumer, nil
}

// Publish simple publisher with unique name and one connect
func Publish(amqpURI, consumerName, exchangeName, exchangeType, routingKey string, msg []byte, headers map[string]interface{}) error {
	consumer, err := GetConsumer(amqpURI, consumerName, &Exchange{Name: exchangeName, Type: exchangeType, Durable: true}, nil, nil)
	if err != nil {
		return err
	}
	if len(headers) > 0 {
		return consumer.PublishWithHeaders(msg, routingKey, headers)
	}
	return consumer.Publish(msg, routingKey)
}

// PublishDirect simple publisher with unique name and one connect
func PublishDirect(amqpURI, consumerName, exchangeName, routingKey string, msg []byte) error {
	return Publish(amqpURI, consumerName, exchangeName, "direct", routingKey, msg, nil)
}

// PublishTopic simple publisher with unique name and one connect
func PublishTopic(amqpURI, consumerName, exchangeName, routingKey string, msg []byte) error {
	return Publish(amqpURI, consumerName, exchangeName, "topic", routingKey, msg, nil)
}

// PublishFanout simple publisher with unique name and one connect
func PublishFanout(amqpURI, consumerName, exchangeName, routingKey string, msg []byte) error {
	return Publish(amqpURI, consumerName, exchangeName, "fanout", routingKey, msg, nil)
}

// PublishHeader simple publisher with unique name and one connect
func PublishHeader(amqpURI, consumerName, exchangeName string, msg []byte, headers map[string]interface{}) error {
	return Publish(amqpURI, consumerName, exchangeName, "header", "", msg, nil)
}

func InitSendUpdate() error {
	consumer, err := NewPublisher(amqpURI, "SendUpdate", Exchange{Name: updateExch, Type: "direct", Durable: true})
	if err != nil {
		return err
	}
	consumers.Store("SendUpdate", consumer)
	return nil
}

// SendUpdate Send rpc update command to services
func SendUpdate(amqpURI, collection, id, method string, data interface{}, options ...map[string]interface{}) error {
	objectJSON, err := json.Marshal(data)
	if err != nil {
		return err
	}
	msg := Update{
		ID:         id,
		Cmd:        method,
		Data:       string(objectJSON),
		Collection: collection,
	}
	if len(options) > 0 {
		opts := options[0]
		if recipientsInt, ok := opts["recipients"]; ok {
			if recipients, ok := recipientsInt.([]string); ok {
				msg.Recipients = recipients
			}
		}
		if userInt, ok := opts["user"]; ok {
			if user, ok := userInt.(string); ok {
				msg.Initiator = user
			}
		}
	}
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	var consumer *Consumer
	consumerInt, ok := consumers.Load("SendUpdate")
	if ok {
		consumer = consumerInt.(*Consumer)
		return consumer.Publish(msgJSON, collection)
	} else {
		return errors.New("publisher SendUpdate not inited")
	}
}

func (c *Consumer) handleDeliveries(deliveries <-chan amqp.Delivery) {
	for {
		logrus.Info(c.logInfo("handle deliveries"))
		go func() {
			for d := range deliveries {
				// routingKey := d.RoutingKey
				c.handlers.Range(func(keyInt, cbInt interface{}) bool {
					// key, ok := keyInt.(string)
					// if !ok {
					// 	return true
					// }
					// if key != "#" && routingKey != key {
					// 	return true
					// }
					cbs, ok := cbInt.([]func(*Delivery))
					if !ok {
						return true
					}
					dv := Delivery(d)
					for i := 0; i < len(cbs); i++ {
						cb := cbs[i]
						cb(&dv)
					}
					return true
				})
				d.Ack(false)
			}
		}()
		if <-c.done != nil {
			logrus.Error(c.logInfo("deliveries channel closed"))
			if len(c.queue.Keys) > 0 {
				c.queue.BindedKeys = make([]string, len(c.queue.Keys))
				copy(c.queue.BindedKeys, c.queue.Keys)
				c.queue.Keys = nil
			}
			deliveries = c.Reconnect(c.queue.BindedKeys)
			continue
		}
	}
}

// AddConsumeHandler add handler for queue consumer by routingKeys
func (c *Consumer) AddConsumeHandler(keys []string, handler func(*Delivery)) {
	for i := 0; i < len(keys); i++ {
		key := keys[i]
		if key == "" {
			key = "#"
		}
		handlersInt, _ := c.handlers.LoadOrStore(key, []func(*Delivery){})
		handlers, ok := handlersInt.([]func(*Delivery))
		if !ok {
			logrus.Error("invalid onUpdates handlers (not []func(*Delivery)) for key: " + key)
			continue
		}
		handlers = append(handlers, handler)
		c.handlers.Store(key, handlers)
	}
}

// GenerateName generate random name for queue and exchange
func GenerateName(prefix string) string {
	queueName := prefix
	if envName != "" {
		queueName += "." + envName
	}
	if consumerTag != "" {
		queueName += "." + consumerTag
	} else {
		queueName += "." + *csxstrings.NewId()
	}
	return queueName
}

// OnUpdates Listener to get models events update, create and delete
func OnUpdates(cb func(data *Delivery), keys []string) {
	if updateExch == "" {
		updateExch = "csx.updates"
	}
	exchange := Exchange{Name: updateExch, Type: "direct", Durable: true}
	queueName := GenerateName("onUpdates")
	queue := Queue{
		Name:        queueName,
		ConsumerTag: queueName,
		AutoDelete:  true,
		Durable:     false,
		Keys:        keys,
	}
	cUpdates, err := GetConsumer(amqpURI, "OnUpdates", &exchange, &queue, nil)
	if err != nil {
		logrus.Error("[OnUpdates] consumer init err: ", err)
		logrus.Warn("[OnUpdates] try reconnect to rabbitmq after ", reconTime)
		time.Sleep(reconTime)
		OnUpdates(cb, keys)
		return
	}

	cUpdates.AddConsumeHandler(keys, cb)
}

//Shutdown channel on set app time to live
func (c *Consumer) Shutdown() error {
	// will close() the deliveries channel
	if c.channel != nil {
		if err := c.channel.Cancel("", true); err != nil {
			logrus.Error(c.logInfo("[Shutdown] consumer cancel err: "), err)
			return err
		}
	}

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			logrus.Error(c.logInfo("[Shutdown] AMQP connection close err: "), err)
			return err
		}
	}

	defer logrus.Warn(c.logInfo("[Shutdown] AMQP shutdown OK"))

	// wait for handle() to exit
	return <-c.done
}
