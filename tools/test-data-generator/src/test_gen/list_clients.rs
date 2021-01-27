// Copyright 2021 Contributors to the Parsec project.
// SPDX-License-Identifier: Apache-2.0
use serde_json::json;
use parsec_interface::operations::{NativeOperation,NativeResult};

use parsec_interface::requests::{Opcode, ResponseStatus};
use crate::test_gen::{TestCase,TestSuite};
use parsec_interface::operations::list_clients;

pub fn create_test_suite() -> Result<TestSuite, std::io::Error> {
    Ok(TestSuite{
        op_code: Opcode::ListClients as u32,
        tests: create_tests()?
    })
}

fn create_tests() -> Result<Vec<TestCase>,std::io::Error> {
    Ok(vec!(
        create_test_good()?,
        create_test_fail()?
    ))
}

fn create_test_good() -> Result<TestCase,std::io::Error> {
    let op_string = base64::encode(super::operation_to_bin(Opcode::Ping, NativeOperation::ListClients(list_clients::Operation { }))?);
    let result = NativeResult::ListClients(list_clients::Result{
        clients: vec!(
            "jim".to_owned(),
            "bob".to_owned()
        )
    });
    let result_string = base64::encode(super::result_to_bin(Opcode::Ping, result, ResponseStatus::Success)?);



    let t = TestCase{
        name: "normal_response".to_owned(),
        request_data: json!({}),
        expected_request_binary: op_string,
        response_binary: result_string,
        expected_response: json!([
            "jim",
            "bob"
            ]
        ),
        expect_success: true
    };
    Ok(t)
}

fn create_test_fail() -> Result<TestCase,std::io::Error> {
    let op_string = base64::encode(super::operation_to_bin(Opcode::Ping, NativeOperation::ListClients(list_clients::Operation { }))?);
    let result = NativeResult::ListClients(list_clients::Result{
        clients: vec!()
    });
    let result_string = base64::encode(super::result_to_bin(Opcode::Ping, result, ResponseStatus::PsaErrorNotSupported)?);



    let t = TestCase{
        name: "fail response".to_owned(),
        request_data: json!({}),
        expected_request_binary: op_string,
        response_binary: result_string,
        expected_response: json!([]
        ),
        expect_success: false
    };
    Ok(t)
}