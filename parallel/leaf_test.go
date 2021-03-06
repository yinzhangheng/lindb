package parallel

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/lindb/lindb/models"
	"github.com/lindb/lindb/pkg/encoding"
	"github.com/lindb/lindb/pkg/option"
	"github.com/lindb/lindb/rpc"
	commonmock "github.com/lindb/lindb/rpc/pbmock/common"
	pb "github.com/lindb/lindb/rpc/proto/common"
	"github.com/lindb/lindb/service"
	"github.com/lindb/lindb/sql/stmt"
	"github.com/lindb/lindb/tsdb"
)

func TestLeafTask_Process_Fail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	taskServerFactory := rpc.NewMockTaskServerFactory(ctrl)
	storageService := service.NewMockStorageService(ctrl)
	executorFactory := NewMockExecutorFactory(ctrl)
	serverStream := commonmock.NewMockTaskService_HandleServer(ctrl)
	mockDatabase := tsdb.NewMockDatabase(ctrl)

	currentNode := models.Node{IP: "1.1.1.3", Port: 8000}
	processor := newLeafTask(currentNode, storageService, executorFactory, taskServerFactory)
	// unmarshal error
	err := processor.Process(context.TODO(), &pb.TaskRequest{PhysicalPlan: nil})
	assert.Equal(t, errUnmarshalPlan, err)

	plan, _ := json.Marshal(&models.PhysicalPlan{
		Leafs: []models.Leaf{{BaseNode: models.BaseNode{Indicator: "1.1.1.4:8000"}}},
	})
	// wrong request
	err = processor.Process(context.TODO(), &pb.TaskRequest{PhysicalPlan: plan})
	assert.Equal(t, errWrongRequest, err)

	plan, _ = json.Marshal(&models.PhysicalPlan{
		Database: "test_db",
		Leafs:    []models.Leaf{{BaseNode: models.BaseNode{Indicator: "1.1.1.3:8000"}}},
	})
	query := stmt.Query{MetricName: "cpu"}
	data := encoding.JSONMarshal(&query)

	// db not exist
	storageService.EXPECT().GetDatabase(gomock.Any()).Return(nil, false)
	err = processor.Process(context.TODO(), &pb.TaskRequest{PhysicalPlan: plan, Payload: data})
	assert.Equal(t, errNoDatabase, err)

	// test get upstream err
	storageService.EXPECT().GetDatabase(gomock.Any()).Return(mockDatabase, true)
	taskServerFactory.EXPECT().GetStream(gomock.Any()).Return(nil)
	err = processor.Process(context.TODO(), &pb.TaskRequest{PhysicalPlan: plan, Payload: data})
	assert.Equal(t, errNoSendStream, err)

	// unmarshal query err
	storageService.EXPECT().GetDatabase(gomock.Any()).Return(mockDatabase, true)
	taskServerFactory.EXPECT().GetStream(gomock.Any()).Return(serverStream)
	err = processor.Process(context.TODO(), &pb.TaskRequest{PhysicalPlan: plan, Payload: []byte{1, 2, 3}})
	assert.Equal(t, errUnmarshalQuery, err)

	// test executor fail
	mockDatabase.EXPECT().GetOption().Return(option.DatabaseOption{Interval: "10s"})
	mockDatabase.EXPECT().ExecutorPool().Return(&tsdb.ExecutorPool{})
	taskServerFactory.EXPECT().GetStream(gomock.Any()).Return(serverStream)
	storageService.EXPECT().GetDatabase(gomock.Any()).Return(mockDatabase, true).AnyTimes()
	exec := NewMockExecutor(ctrl)
	exec.EXPECT().Execute()
	executorFactory.EXPECT().NewStorageExecuteContext(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	executorFactory.EXPECT().NewStorageExecutor(gomock.Any(), gomock.Any(), gomock.Any()).Return(exec)
	err = processor.Process(context.TODO(), &pb.TaskRequest{PhysicalPlan: plan, Payload: data})
	assert.NoError(t, err)
}

func TestLeafProcessor_Process(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	taskServerFactory := rpc.NewMockTaskServerFactory(ctrl)
	storageService := service.NewMockStorageService(ctrl)
	executorFactory := NewMockExecutorFactory(ctrl)

	currentNode := models.Node{IP: "1.1.1.3", Port: 8000}
	processor := newLeafTask(currentNode, storageService, executorFactory, taskServerFactory)
	mockDatabase := tsdb.NewMockDatabase(ctrl)
	plan, _ := json.Marshal(&models.PhysicalPlan{
		Database: "test_db",
		Leafs:    []models.Leaf{{BaseNode: models.BaseNode{Indicator: "1.1.1.3:8000"}}},
	})
	query := stmt.Query{MetricName: "cpu"}
	data := encoding.JSONMarshal(&query)

	mockDatabase.EXPECT().GetOption().Return(option.DatabaseOption{Interval: "10s"})
	mockDatabase.EXPECT().ExecutorPool().Return(&tsdb.ExecutorPool{})
	storageService.EXPECT().GetDatabase(gomock.Any()).Return(mockDatabase, true)

	serverStream := commonmock.NewMockTaskService_HandleServer(ctrl)
	taskServerFactory.EXPECT().GetStream(gomock.Any()).Return(serverStream)
	exec := NewMockExecutor(ctrl)
	exec.EXPECT().Execute()
	executorFactory.EXPECT().NewStorageExecutor(gomock.Any(), gomock.Any(), gomock.Any()).Return(exec)
	executorFactory.EXPECT().NewStorageExecuteContext(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	err := processor.Process(context.TODO(), &pb.TaskRequest{PhysicalPlan: plan, Payload: data})
	assert.NoError(t, err)
}

func TestLeafTask_Suggest_Process(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	taskServerFactory := rpc.NewMockTaskServerFactory(ctrl)
	storageService := service.NewMockStorageService(ctrl)
	executorFactory := NewMockExecutorFactory(ctrl)
	exec := NewMockMetadataExecutor(ctrl)
	executorFactory.EXPECT().NewMetadataStorageExecutor(gomock.Any(), gomock.Any(), gomock.Any()).Return(exec).AnyTimes()

	currentNode := models.Node{IP: "1.1.1.3", Port: 8000}
	processor := newLeafTask(currentNode, storageService, executorFactory, taskServerFactory)
	mockDatabase := tsdb.NewMockDatabase(ctrl)
	plan, _ := json.Marshal(&models.PhysicalPlan{
		Database: "test_db",
		Leafs:    []models.Leaf{{BaseNode: models.BaseNode{Indicator: "1.1.1.3:8000"}}},
	})
	storageService.EXPECT().GetDatabase(gomock.Any()).Return(mockDatabase, true).AnyTimes()
	serverStream := commonmock.NewMockTaskService_HandleServer(ctrl)
	taskServerFactory.EXPECT().GetStream(gomock.Any()).Return(serverStream).AnyTimes()

	// test unmarshal err
	err := processor.Process(context.TODO(), &pb.TaskRequest{
		PhysicalPlan: plan,
		RequestType:  pb.RequestType_Metadata,
		Payload:      []byte{1, 2, 3}})
	assert.Error(t, err)

	// test execute err
	data := encoding.JSONMarshal(&stmt.Metadata{})
	exec.EXPECT().Execute().Return(nil, fmt.Errorf("err"))
	err = processor.Process(context.TODO(), &pb.TaskRequest{
		PhysicalPlan: plan,
		RequestType:  pb.RequestType_Metadata,
		Payload:      data})
	assert.Error(t, err)

	// test send result err
	exec.EXPECT().Execute().Return([]string{"a"}, nil)
	serverStream.EXPECT().Send(gomock.Any()).Return(fmt.Errorf("err"))
	err = processor.Process(context.TODO(), &pb.TaskRequest{
		PhysicalPlan: plan,
		RequestType:  pb.RequestType_Metadata,
		Payload:      data})
	assert.Error(t, err)

	// normal case
	exec.EXPECT().Execute().Return([]string{"a"}, nil)
	serverStream.EXPECT().Send(gomock.Any()).Return(nil)
	err = processor.Process(context.TODO(), &pb.TaskRequest{
		PhysicalPlan: plan,
		RequestType:  pb.RequestType_Metadata,
		Payload:      data})
	assert.NoError(t, err)
}
