package sqsextendedclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// assert not mutating inputs

type mockSQSClient struct {
	*mock.Mock
	SQSClient
}

func (m *mockSQSClient) SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*sqs.SendMessageOutput), args.Error(1)
}

func (m *mockSQSClient) SendMessageBatch(ctx context.Context, params *sqs.SendMessageBatchInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageBatchOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*sqs.SendMessageBatchOutput), args.Error(1)
}

func (m *mockSQSClient) ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*sqs.ReceiveMessageOutput), args.Error(1)
}

func (m *mockSQSClient) DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*sqs.DeleteMessageOutput), args.Error(1)
}

func (m *mockSQSClient) DeleteMessageBatch(ctx context.Context, params *sqs.DeleteMessageBatchInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*sqs.DeleteMessageBatchOutput), args.Error(1)
}

type mockS3Client struct {
	*mock.Mock
}

func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*s3.PutObjectOutput), args.Error(1)
}

func (m *mockS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*s3.GetObjectOutput), args.Error(1)
}

func (m *mockS3Client) DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*s3.DeleteObjectOutput), args.Error(1)
}

func (m *mockS3Client) DeleteObjects(ctx context.Context, params *s3.DeleteObjectsInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*s3.DeleteObjectsOutput), args.Error(1)
}

func TestNewClient(t *testing.T) {
	c, err := New(nil, nil)
	assert.Nil(t, err)

	// ensure defaults are set correctly
	assert.Equal(t, int64(262144), c.messageSizeThreshold)
	assert.Equal(t, "software.amazon.payloadoffloading.PayloadS3Pointer", c.pointerClass)
	assert.Equal(t, "ExtendedPayloadSize", c.reservedAttrName)
}

func TestNewClientOptions(t *testing.T) {
	c, err := New(
		nil,
		nil,
		WithAlwaysS3(true),
		WithMessageSizeThreshold(123),
		WithPointerClass("pointer.class"),
		WithReservedAttributeName("Reserved"),
		WithS3BucketName("BUCKET!"),
	)

	assert.Nil(t, err)

	// ensure options are set correctly
	assert.Equal(t, true, c.alwaysThroughS3)
	assert.Equal(t, int64(123), c.messageSizeThreshold)
	assert.Equal(t, "pointer.class", c.pointerClass)
	assert.Equal(t, "Reserved", c.reservedAttrName)
	assert.Equal(t, "BUCKET!", c.bucketName)
}

func TestNewClientOptionsFailure(t *testing.T) {
	c, err := New(
		nil,
		nil,
		func(c *Client) error { return errors.New("boom") },
	)

	assert.ErrorContains(t, err, "boom")
	assert.Nil(t, c)
}

func TestAttributeSize(t *testing.T) {
	c, err := New(nil, nil)
	assert.Nil(t, err)

	assert.Equal(t, int64(26), c.attributeSize(map[string]types.MessageAttributeValue{
		"testing_strings": {
			StringValue: aws.String("some string"),
		},
	}))

	assert.Equal(t, int64(20), c.attributeSize(map[string]types.MessageAttributeValue{
		"testing_binary": {
			BinaryValue: []byte{1, 2, 3, 4, 5, 6},
		},
	}))

	assert.Equal(t, int64(47), c.attributeSize(map[string]types.MessageAttributeValue{
		"binary_attr": {
			BinaryValue: []byte{1, 2, 3, 4, 5, 6},
		},
		"string_attr1": {
			StringValue: aws.String("str"),
		},
		"string_attr2": {
			StringValue: aws.String("str"),
		},
	}))
}

func TestMessageExceedThreshold(t *testing.T) {
	c, err := New(nil, nil, WithMessageSizeThreshold(10))
	assert.Nil(t, err)

	assert.False(t, c.messageExceedsThreshold(
		aws.String("nnnnnnnnnn"),
		map[string]types.MessageAttributeValue{},
	))

	assert.True(t, c.messageExceedsThreshold(
		aws.String("nnnnnnnnnnn"),
		map[string]types.MessageAttributeValue{},
	))

	assert.False(t, c.messageExceedsThreshold(
		aws.String("nnnnn"),
		map[string]types.MessageAttributeValue{
			"str": {StringValue: aws.String("hi")},
		},
	))

	assert.True(t, c.messageExceedsThreshold(
		aws.String("nnnnnn"),
		map[string]types.MessageAttributeValue{
			"str": {StringValue: aws.String("hi")},
		},
	))
}

func TestS3PointerMarshal(t *testing.T) {
	p := s3Pointer{
		S3BucketName: "some-bucket",
		S3Key:        "some-key",
		class:        "com.james.testing.Pointer",
	}

	asBytes, err := p.MarshalJSON()
	assert.Nil(t, err)
	assert.Equal(t, `["com.james.testing.Pointer",{"s3BucketName":"some-bucket","s3Key":"some-key"}]`, string(asBytes))
}

func TestS3PointerUnmarshal(t *testing.T) {
	str := []byte(`["com.james.testing.Pointer",{"s3BucketName":"some-bucket","s3Key":"some-key"}]`)

	var p s3Pointer
	err := p.UnmarshalJSON(str)
	assert.Nil(t, err)
	assert.Equal(t, s3Pointer{
		S3BucketName: "some-bucket",
		S3Key:        "some-key",
		class:        "com.james.testing.Pointer",
	}, p)
}

func TestS3PointerUnmarshalError(t *testing.T) {
	var p s3Pointer
	err := p.UnmarshalJSON([]byte(""))
	assert.NotNil(t, err)
}

func TestS3PointerUnmarshalInvalidLength(t *testing.T) {
	str := []byte(`["com.james.testing.Pointer",{"s3BucketName":"some-bucket","s3Key":"some-key"}, "bonus!"]`)

	var p s3Pointer
	err := p.UnmarshalJSON(str)
	assert.ErrorContains(t, err, "invalid pointer format, expected length 2, but received [3]")
}

func TestSendMessage(t *testing.T) {
	key := new(string)

	ms3c := &mockS3Client{&mock.Mock{}}
	ms3c.On(
		"PutObject",
		mock.Anything,
		mock.MatchedBy(func(params *s3.PutObjectInput) bool {
			key = params.Key

			assert.Greater(t, len(*params.Key), 0)
			assert.Equal(t, "test_bucket", *params.Bucket)

			asBytes, err := io.ReadAll(params.Body)
			assert.Nil(t, err)
			assert.Equal(t, "testing body", string(asBytes))

			return true
		}),
		mock.Anything).
		Return(&s3.PutObjectOutput{}, nil)

	msqsc := &mockSQSClient{Mock: &mock.Mock{}}
	msqsc.On(
		"SendMessage",
		mock.Anything,
		mock.MatchedBy(func(params *sqs.SendMessageInput) bool {
			assert.Equal(t, "12", *params.MessageAttributes["ExtendedPayloadSize"].StringValue)
			assert.Equal(t, "hi", *params.MessageAttributes["testing_attribute"].StringValue)
			assert.Equal(t, fmt.Sprintf(`["software.amazon.payloadoffloading.PayloadS3Pointer",{"s3BucketName":"test_bucket","s3Key":"%s"}]`, *key), *params.MessageBody)

			return true
		}),
		mock.Anything).
		Return(&sqs.SendMessageOutput{}, nil)

	c, err := New(msqsc, ms3c, WithAlwaysS3(true), WithS3BucketName("test_bucket"))
	assert.Nil(t, err)

	_, err = c.SendMessage(context.Background(), &sqs.SendMessageInput{
		MessageBody: aws.String("testing body"),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"testing_attribute": {StringValue: aws.String("hi")},
		},
	})

	assert.Nil(t, err)
}

func TestSendMessageS3Failure(t *testing.T) {
	ms3c := &mockS3Client{&mock.Mock{}}
	ms3c.On("PutObject", mock.Anything, mock.Anything, mock.Anything).Return(&s3.PutObjectOutput{}, errors.New("boom"))

	c, err := New(nil, ms3c, WithAlwaysS3(true))
	assert.Nil(t, err)

	_, err = c.SendMessage(context.Background(), &sqs.SendMessageInput{MessageBody: aws.String("testing body")})

	assert.ErrorContains(t, err, "unable to upload large payload to s3")
}

func TestSendMessageMarshalFailure(t *testing.T) {
	jsonMarshal = func(v any) ([]byte, error) { return nil, errors.New("boom") }
	defer func() { jsonMarshal = json.Marshal }()

	ms3c := &mockS3Client{&mock.Mock{}}
	ms3c.On("PutObject", mock.Anything, mock.Anything, mock.Anything).Return(&s3.PutObjectOutput{}, nil)

	c, err := New(nil, ms3c, WithAlwaysS3(true))
	assert.Nil(t, err)

	_, err = c.SendMessage(context.Background(), &sqs.SendMessageInput{MessageBody: aws.String("testing body")})

	assert.ErrorContains(t, err, "unable to marshal S3 pointer")
}

func TestParseExtendedReceiptHandle(t *testing.T) {
	bucket, key, handle := parseExtendedReceiptHandle("-..s3BucketName..-some-bucket-..s3BucketName..--..s3Key..-some-key-..s3Key..-abcdefg")
	assert.Equal(t, "some-bucket", bucket)
	assert.Equal(t, "some-key", key)
	assert.Equal(t, "abcdefg", handle)
}

func TestParseExtendedReceiptHandleFailure(t *testing.T) {
	bucket, key, handle := parseExtendedReceiptHandle("nonExtendedHandle")
	assert.Equal(t, "", bucket)
	assert.Equal(t, "", key)
	assert.Equal(t, "", handle)
}
