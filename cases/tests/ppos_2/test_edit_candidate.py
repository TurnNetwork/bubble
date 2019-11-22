# -*- coding: utf-8 -*-

from tests.lib.utils import *
import pytest
import allure


@allure.title("Modify node information")
@pytest.mark.P0
@pytest.mark.compatibility
def test_MPI_001_002(client_new_node_obj):
    """
    Modify node information
    :param client_new_node_obj:
    :return:
    """
    external_id = "ID1"
    node_name = "LIDA"
    website = "WEBSITE"
    details = "talent"
    address, pri_key = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                             10 ** 18 * 10000000)
    result = client_new_node_obj.staking.create_staking(0, address, address)
    assert_code(result, 0)
    result = client_new_node_obj.ppos.editCandidate(address, client_new_node_obj.node.node_id, external_id,
                                                    node_name, website, details, pri_key)
    assert_code(result, 0)
    result = client_new_node_obj.ppos.getCandidateInfo(client_new_node_obj.node.node_id)
    log.info(result)
    assert result["Ret"]["ExternalId"] == external_id
    assert result["Ret"]["NodeName"] == node_name
    assert result["Ret"]["Website"] == website
    assert result["Ret"]["Details"] == details


@allure.title("Node becomes consensus validator when modifying revenue address")
@pytest.mark.P2
def test_MPI_003(client_new_node_obj_list):
    """
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj_list[0]
    address, _ = client.economic.account.generate_account(client.node.web3,
                                                                       10 ** 18 * 10000000)
    value = client.economic.create_staking_limit * 2
    result = client.staking.create_staking(0, address, address, amount=value)
    assert_code(result, 0)
    msg = client.ppos.getCandidateInfo(client.node.node_id)
    log.info(msg)
    log.info("Next settlement period")
    client.economic.wait_settlement_blocknum(client.node)
    log.info("Next consensus cycle")
    client.economic.wait_consensus_blocknum(client.node)
    validator_list = get_pledge_list(client.ppos.getValidatorList)
    log.info("validator_list:{}".format(validator_list))
    log.info("new node{}".format(client.node.node_id))
    msg = client.ppos.getValidatorList()
    log.info("validator_list info{}".format(msg))
    assert client.node.node_id in validator_list
    result = client.staking.edit_candidate(address, address)
    assert_code(result, 0)


@allure.title("The original verifier beneficiary's address modifies the ordinary address")
@pytest.mark.P1
def test_MPI_004(client_consensus_obj):
    """
    :param client_consensus_obj:
    :return:
    """
    address, _ = client_consensus_obj.economic.account.generate_account(client_consensus_obj.node.web3,
                                                                        10 ** 18 * 10000000)
    INCENTIVEPOOL_ADDRESS = client_consensus_obj.economic.cfg.INCENTIVEPOOL_ADDRESS
    DEVELOPER_FOUNDATAION_ADDRESS = client_consensus_obj.economic.cfg.DEVELOPER_FOUNDATAION_ADDRESS

    result = client_consensus_obj.staking.edit_candidate(DEVELOPER_FOUNDATAION_ADDRESS, address)
    log.info(result)
    msg = client_consensus_obj.ppos.getCandidateInfo(client_consensus_obj.node.node_id)
    log.info(msg)
    assert msg["Ret"]["BenefitAddress"] == INCENTIVEPOOL_ADDRESS


@allure.title("The beneficiary address of the non-initial verifier is changed to the incentive pool address")
@pytest.mark.P1
def test_MPI_005_006(client_new_node_obj):
    """
    005:The beneficiary address of the non-initial verifier is changed to the incentive pool address
    006:and then to the ordinary address
    :param client_new_node_obj:
    :param get_generate_account:
    :return:
    """
    address, _ = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                       10 ** 18 * 10000000)
    INCENTIVEPOOL_ADDRESS = client_new_node_obj.economic.cfg.INCENTIVEPOOL_ADDRESS
    result = client_new_node_obj.staking.create_staking(0, address, address)
    assert_code(result, 0)
    log.info("Change to excitation pool address")
    result = client_new_node_obj.staking.edit_candidate(address, INCENTIVEPOOL_ADDRESS)
    log.info(result)
    assert_code(result, 0)
    msg = client_new_node_obj.ppos.getCandidateInfo(client_new_node_obj.node.node_id)
    log.info(msg)
    assert msg["Ret"]["BenefitAddress"] == INCENTIVEPOOL_ADDRESS

    result = client_new_node_obj.staking.edit_candidate(address, address)
    log.info(result)
    assert_code(result, 0)
    msg = client_new_node_obj.ppos.getCandidateInfo(client_new_node_obj.node.node_id)
    log.info(msg)
    assert msg["Ret"]["BenefitAddress"] == INCENTIVEPOOL_ADDRESS


@allure.title("Edit wallet address as legal")
@pytest.mark.P1
def test_MPI_007(client_new_node_obj, client_noconsensus_obj):
    """
    :param client_new_node_obj:
    :param client_noconsensus_obj:
    :return:
    """
    address1, _ = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                        10 ** 18 * 10000000)
    log.info(address1)
    result = client_new_node_obj.staking.create_staking(0, address1, address1)
    assert_code(result, 0)
    account = client_noconsensus_obj.economic.account
    node = client_noconsensus_obj.node
    address2, _ = account.generate_account(node.web3, 10 ** 18 * 20000000)
    result = client_new_node_obj.staking.edit_candidate(address1, address2)
    log.info(address2)
    log.info(result)
    assert_code(result, 0)
    msg = client_new_node_obj.ppos.getCandidateInfo(client_new_node_obj.node.node_id)
    log.info(msg)
    assert client_new_node_obj.node.web3.toChecksumAddress(msg["Ret"]["BenefitAddress"]) == address2


@allure.title("It is illegal to edit wallet addresses")
@pytest.mark.P1
def test_MPI_008(client_new_node_obj):
    """
    :param client_new_node_obj:
    :return:
    """
    address1, _ = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                        10 ** 18 * 10000000)
    log.info(address1)
    result = client_new_node_obj.staking.create_staking(0, address1, address1)
    assert_code(result, 0)
    address2 = "0x111111111111111111111111111111"
    status = 0
    try:
        result = client_new_node_obj.staking.edit_candidate(address1, address2)
        log.info(result)
    except BaseException:
        status = 1
    assert status == 1


@allure.title("Insufficient gas to initiate modification node")
@pytest.mark.P3
def test_MPI_009(client_new_node_obj):
    """
    :param client_new_node_obj:
    :return:
    """

    address, _ = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                       10 ** 18 * 10000000)
    result = client_new_node_obj.staking.create_staking(0,address,address)
    assert_code(result,0)
    cfg = {"gas": 1}
    status = 0
    try:
        result = client_new_node_obj.staking.edit_candidate(address, address, transaction_cfg=cfg)
        log.info(result)
    except BaseException:
        status = 1
    assert status == 1


@allure.title("Insufficient balance to initiate the modification node")
@pytest.mark.P3
def test_MPI_010(client_new_node_obj):
    """
    :param client_new_node_obj:
    :return:
    """

    account = client_new_node_obj.economic.account
    client= client_new_node_obj
    node = client.node
    address, _ = account.generate_account(node.web3, 10 ** 18 * 10000000)
    result = client.staking.create_staking(0,address,address)
    assert_code(result,0)
    balance = node.eth.getBalance(address)
    status = 0
    try:
        result = client.staking.edit_candidate(address, address,transaction_cfg={"gasPrice":balance})
        log.info(result)
    except BaseException:
        status = 1
    assert status == 1


@allure.title("During the hesitation period, withdraw pledge and modify node information")
@pytest.mark.P1
def test_MPI_011(client_new_node_obj):
    """
    :param client_new_node_obj:
    :return:
    """
    address, _ = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                       10 ** 18 * 10000000)
    result = client_new_node_obj.staking.create_staking(0, address, address)
    assert_code(result, 0)
    result = client_new_node_obj.staking.withdrew_staking(address)
    log.info(result)
    result = client_new_node_obj.staking.edit_candidate(address, address)
    log.info(result)
    assert_code(result, 301102)


@allure.title("Lock period exit pledge, modify node information")
@pytest.mark.P2
def test_MPI_012_013(client_new_node_obj):
    """
    012:Lock period exit pledge, modify node information
    013:After the lockout pledge is complete, the node information shall be modified
    :param client_new_node_obj:
    :return:
    """
    address, _ = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                       10 ** 18 * 10000000)
    result = client_new_node_obj.staking.create_staking(0, address, address)
    assert_code(result, 0)
    log.info("Next settlement period")
    client_new_node_obj.economic.wait_settlement_blocknum(client_new_node_obj.node)
    log.info("The lock shall be depledged at regular intervals")
    result = client_new_node_obj.staking.withdrew_staking(address)
    assert_code(result, 0)
    msg = client_new_node_obj.ppos.getCandidateInfo(client_new_node_obj.node.node_id)
    log.info(msg)
    assert msg["Code"] == 0
    result = client_new_node_obj.staking.edit_candidate(address, address)
    log.info(result)
    assert_code(result, 301103)
    log.info("Next two settlement period")
    client_new_node_obj.economic.wait_settlement_blocknum(client_new_node_obj.node, number=2)
    msg = client_new_node_obj.ppos.getCandidateInfo(client_new_node_obj.node.node_id)
    log.info(msg)
    assert msg["Code"] == 301204
    result = client_new_node_obj.staking.edit_candidate(address, address)
    log.info(result)
    assert_code(result, 301102)


@allure.title("Non-verifier, modify node information")
@pytest.mark.P3
def test_MPI_014(client_new_node_obj):
    """
    Non-verifier, modify node information
    :param client_new_node_obj:
    :return:
    """
    external_id = "ID1"
    node_name = "LIDA"
    website = "WEBSITE"
    details = "talent"
    illegal_nodeID = "7ee3276fd6b9c7864eb896310b5393324b6db785a2528c00cc28ca8c" \
                     "3f86fc229a86f138b1f1c8e3a942204c03faeb40e3b22ab11b8983c35dc025de42865990"
    address, pri_key = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                             10 ** 18 * 10000000)
    result = client_new_node_obj.staking.create_staking(0, address, address)
    assert_code(result, 0)
    result = client_new_node_obj.ppos.editCandidate(address, illegal_nodeID, external_id,
                                                    node_name, website, details, pri_key)
    log.info(result)
    assert_code(result, 301102)


@allure.title("Candidates whose commissions have been penalized are still frozen")
@pytest.mark.P2
def test_MPI_015_016(client_new_node_obj, client_consensus_obj):
    """
    015:Candidates whose commissions have been penalized are still frozen
    016:A candidate whose mandate has expired after a freeze period
    :param client_new_node_obj:
    :return:
    """
    address, pri_key = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                             10 ** 18 * 10000000)

    value = client_new_node_obj.economic.create_staking_limit * 2
    result = client_new_node_obj.staking.create_staking(0, address, address, amount=value)
    assert_code(result, 0)
    log.info("Close one node")
    client_new_node_obj.node.stop()
    node = client_consensus_obj.node
    log.info("The next two periods")
    client_new_node_obj.economic.wait_settlement_blocknum(node, number=2)
    log.info("Restart the node")
    client_new_node_obj.node.start()
    result = client_new_node_obj.staking.edit_candidate(address, address)
    log.info(result)
    assert_code(result, 301103)
    log.info("Next settlement period")
    client_new_node_obj.economic.wait_settlement_blocknum(client_new_node_obj.node)
    time.sleep(20)
    result = client_new_node_obj.staking.edit_candidate(address, address)
    log.info(result)
    assert_code(result, 301102)


@allure.title("The length of the overflow")
@pytest.mark.P2
def test_MPI_017(client_new_node_obj):
    external_id = "11111111111111111111111111111111111111111111111111111111111111111111111111111111111"
    node_name = "1111111111111111111111111111111111111111111111111111111111111111111111111111111111111"
    website = "1111111111111111111111111111111111111111111111111111111111111111111111111111111111111111 "
    details = "1111111111111111111111111111111111111111111111111111111111111111111111111111111111111111 "

    address, pri_key = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                             10 ** 18 * 10000000)

    result = client_new_node_obj.ppos.editCandidate(address, client_new_node_obj.node.node_id,
                                                    external_id, node_name, website, details, pri_key)
    log.info(result)
    assert_code(result, 301102)
