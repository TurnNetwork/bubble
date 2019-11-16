from decimal import Decimal

import allure
import pytest
import time
from common.log import log
from client_sdk_python import Web3
from tests.lib.utils import get_pledge_list, get_block_count_number, assert_code
from common.key import generate_key


def calculate(big_int, mul):
    return int(Decimal(str(big_int)) * Decimal(mul))


@pytest.fixture()
def staking_client(client_new_node_obj):
    amount = calculate(client_new_node_obj.economic.create_staking_limit, 5)
    staking_amount = calculate(client_new_node_obj.economic.create_staking_limit, 2)
    staking_address, _ = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3, amount)
    delegate_address, _ = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                                client_new_node_obj.economic.add_staking_limit * 2)
    client_new_node_obj.staking.create_staking(0, staking_address, staking_address, amount=staking_amount)
    setattr(client_new_node_obj, "staking_address", staking_address)
    setattr(client_new_node_obj, "delegate_address", delegate_address)
    setattr(client_new_node_obj, "amount", amount)
    setattr(client_new_node_obj, "staking_amount", staking_amount)
    yield client_new_node_obj
    if not client_new_node_obj.economic.env.running:
        client_new_node_obj.economic.env.deploy_all()


@allure.title("验证人申请退回质押金（犹豫期）")
@pytest.mark.P0
def test_RV_001(staking_client):
    """
    The certifier applies for a refund of the quality deposit (hesitation period)
    """
    client = staking_client
    staking_address = client.staking_address
    node = client.node
    balance_before = node.eth.getBalance(staking_address)
    log.info("Corresponding wallet balance {}".format(balance_before))
    client.staking.withdrew_staking(staking_address)
    balance_after = node.eth.getBalance(staking_address)
    log.info("Node 4 exits the pledge wallet balance {}".format(balance_after))
    assert balance_after > balance_before, "After exiting the pledge, the wallet balance has not increased"
    log.info(
        "Because the value of gas consumed by the pledge is greater than the value of the gas that cancels the pledge")
    assert balance_after > client.amount - 10 ** 18
    node_list = get_pledge_list(node.ppos.getCandidateList)
    assert node.node_id not in node_list, "Verify that the node exits abnormally"


@allure.title("验证人退回质押金（锁定期）")
@pytest.mark.P1
def test_RV_002(staking_client):
    """
    The certifier refunds the quality deposit (unreachable unlockable period)
    Pledge becomes the next cycle verifier, after exiting, exit in the next settlement cycle
    """
    client = staking_client
    staking_address = client.staking_address
    node = client.node
    economic = client.economic
    staking_address_balance = node.eth.getBalance(staking_address)
    log.info(staking_address_balance)
    economic.wait_settlement_blocknum(node)
    log.info("Query the certifier for the second billing cycle")
    node_list = get_pledge_list(client.ppos.getVerifierList)
    log.info(node_list)
    assert node.node_id in node_list
    log.info("The node applies for a return during the lockout period in the second settlement cycle.")
    client.staking.withdrew_staking(staking_address)
    """Initiation of returning consumes a certain amount of gas"""
    staking_address_balance_1 = node.eth.getBalance(staking_address)
    log.info(staking_address_balance_1)
    log.info("Enter the third billing cycle")
    economic.wait_settlement_blocknum(node)
    staking_address_balance_2 = node.eth.getBalance(staking_address)
    log.info(staking_address_balance_2)
    node_list = get_pledge_list(client.ppos.getVerifierList)
    log.info(node_list)
    assert node.node_id not in node_list
    log.info("Enter the 4th billing cycle")
    economic.wait_settlement_blocknum(node)
    msg = client.ppos.getCandidateInfo(node.node_id)
    log.info(msg)
    staking_address_balance_3 = node.eth.getBalance(staking_address)
    log.info(staking_address_balance_3)
    log.info(staking_address_balance_3 - staking_address_balance_1)
    assert staking_address_balance_3 - staking_address_balance_1 > client.staking_amount, "The amount of the returned transaction should be greater than the amount of the returned deposit."


@allure.title("验证人申请退回质押金（犹豫期+锁定期）")
@pytest.mark.P1
def test_RV_003(staking_client):
    """
    The certifier applies for a refund of the quality deposit (hesitation period + lock-up period)
    """
    client = staking_client
    staking_address = client.staking_address
    node = client.node
    economic = client.economic
    log.info("Enter the next cycle")
    economic.wait_settlement_blocknum(node)
    msg = client.staking.increase_staking(0, staking_address)
    assert_code(msg, 0)
    msg = node.ppos.getCandidateInfo(node.node_id)
    log.info("Pledge information {}".format(msg))
    assert msg["Ret"][
               "Shares"] == client.staking_amount + economic.add_staking_limit, "Expected display of the amount of deposit + increase in holding amount"
    assert msg["Ret"]["Released"] == client.staking_amount, "Expected display of the amount of the deposit"
    assert msg["Ret"][
               "ReleasedHes"] == economic.add_staking_limit, "Expected increase in holdings is shown during the hesitation period"
    block_reward, staking_reward = economic.get_current_year_reward(node)

    balance = node.eth.getBalance(staking_address)
    log.info("Initiate a pre-retardment balance{}".format(balance))

    log.info("Initiation of the return pledge in the second cycle")
    msg = client.staking.withdrew_staking(staking_address)
    assert_code(msg, 0)
    msg = node.ppos.getCandidateInfo(node.node_id)
    log.info("Initiate a refund after pledge information{}".format(msg))
    assert msg["Ret"][
               "ReleasedHes"] == 0, "The amount of expected increase in shareholding has been returned, showing 0"
    balance1 = node.eth.getBalance(client.staking_address)
    log.info(balance1)
    log.info("Enter the 3rd cycle")
    economic.wait_settlement_blocknum(node, 2)

    balance2 = node.eth.getBalance(staking_address)
    log.info(balance2)

    block_number = get_block_count_number(node, economic.settlement_size * 3)
    sum_block_reward = calculate(block_reward, block_number)
    reward_sum = sum_block_reward + staking_reward
    log.info("Total amount of reward {}".format(reward_sum))
    assert balance1 + reward_sum + client.staking_amount == balance2, "The bonus amount is abnormal"


@allure.title("自由账户质押+锁仓账户增持(犹豫期退质押)")
@pytest.mark.P1
def test_RV_004(staking_client):
    client = staking_client
    staking_address = client.staking_address
    node = client.node
    economic = client.economic
    log.info("Create a lockout plan")
    lockup_amount = economic.add_staking_limit * 2
    plan = [{'Epoch': 1, 'Amount': lockup_amount}]
    msg = client.restricting.createRestrictingPlan(staking_address, plan,
                                                   economic.account.account_with_money["address"])
    assert_code(msg, 0)
    locked_info = client.ppos.getRestrictingInfo(staking_address)
    log.info(locked_info)
    before_create_balance = client.amount
    log.info("Initiate the balance before the pledge {}".format(before_create_balance))

    msg = client.staking.increase_staking(1, staking_address)
    assert_code(msg, 0)
    msg = client.ppos.getCandidateInfo(node.node_id)
    log.info("Query pledge {}".format(msg))
    log.info("Initiating a pledge")
    msg = client.staking.withdrew_staking(staking_address)
    assert_code(msg, 0)

    after_balance_1 = node.eth.getBalance(staking_address)
    log.info("Hesitant period to initiate a refunded balance{}".format(after_balance_1))
    """The balance after return is definitely less than the balance before the pledge, the consumption is less than 1 eth"""
    assert before_create_balance - after_balance_1 < Web3.toWei(1, "ether"), "The returned amount is abnormal"
    locked_info = client.ppos.getRestrictingInfo(staking_address)
    log.info(locked_info)

    msg = client.ppos.getCandidateInfo(node.node_id)
    assert_code(msg, 301204)
    log.info("Enter the next cycle")
    economic.wait_settlement_blocknum(node)
    locked_info = client.ppos.getRestrictingInfo(staking_address)
    log.info(locked_info)
    after_account = node.eth.getBalance(staking_address)
    log.info("Account balance after the lockout is released{}".format(after_account))
    assert after_account - after_balance_1 == lockup_amount, "The amount of the lockout returned is abnormal."


@allure.title("自由账户质押+锁仓账户增持(锁定期退质押)")
@pytest.mark.P1
def test_RV_005(staking_client):
    client = staking_client
    node = client.node
    staking_address = client.staking_address
    economic = client.economic
    log.info("Create a lockout plan")
    lockup_amount = economic.add_staking_limit * 2
    plan = [{'Epoch': 1, 'Amount': lockup_amount}]
    msg = client.restricting.createRestrictingPlan(staking_address, plan,
                                                   economic.account.account_with_money["address"])
    assert_code(msg, 0)
    locked_info = client.ppos.getRestrictingInfo(staking_address)
    log.info(locked_info)
    before_create_balance = client.amount
    log.info("Initiate the balance before the pledge {}".format(before_create_balance))

    msg = client.staking.increase_staking(1, staking_address)
    assert_code(msg, 0)
    economic.wait_settlement_blocknum(node)

    msg = client.ppos.getCandidateInfo(node.node_id)
    log.info("Query pledge {}".format(msg))
    assert msg["Ret"]["Shares"] == client.staking_amount + economic.add_staking_limit
    assert msg["Ret"]["Released"] == client.staking_amount
    assert msg["Ret"]["RestrictingPlan"] == economic.add_staking_limit

    block_reward, staking_reward = economic.get_current_year_reward(node)
    msg = client.staking.withdrew_staking(staking_address)
    assert_code(msg, 0)
    balance_withdrew = node.eth.getBalance(staking_address)
    log.info("The second cycle initiated the revocation of the balance{}".format(balance_withdrew))
    log.info("Enter the 3rd cycle")
    economic.wait_settlement_blocknum(node)

    balance_settlement = node.eth.getBalance(staking_address)
    log.info("The balance after launching the revocation in the third cycle{}".format(balance_settlement))

    log.info("Enter the 4th cycle")
    economic.wait_settlement_blocknum(node, 1)

    balance_settlement_2 = node.eth.getBalance(staking_address)
    log.info("The balance after the withdrawal of the fourth cycle {}".format(balance_settlement_2))

    """Calculate block reward + pledge reward"""
    log.info("The following is the number of blocks to get the node")
    block_number = get_block_count_number(node, economic.settlement_size * 3)
    sum_block_reward = calculate(block_reward, block_number)
    reward_sum = sum_block_reward + staking_reward
    log.info("Total amount of reward {}".format(reward_sum))
    assert before_create_balance + reward_sum + lockup_amount - balance_settlement_2 < Web3.toWei(1,
                                                                                                  "ether"), "After the expected result unlock period, the money has been refunded + the block reward + pledge reward"


@allure.title("自由账户质押+锁仓账户增持(都存在犹豫期+锁定期)")
@pytest.mark.P1
def test_RV_006(staking_client):
    client = staking_client
    staking_address = client.staking_address
    node = client.node
    economic = client.economic
    log.info("Create a lockout plan")
    lockup_amount = economic.add_staking_limit * 5
    plan = [{'Epoch': 3, 'Amount': lockup_amount}]
    msg = client.restricting.createRestrictingPlan(staking_address, plan,
                                                   economic.account.account_with_money["address"])
    assert_code(msg, 0), "Creating a lockout plan failed"
    locked_info = client.ppos.getRestrictingInfo(staking_address)
    log.info(locked_info)

    msg = client.staking.increase_staking(1, staking_address)
    assert_code(msg, 0)
    log.info("Enter the second cycle")
    economic.wait_settlement_blocknum(node)

    msg = client.staking.increase_staking(1, staking_address)
    assert_code(msg, 0)
    msg = client.staking.increase_staking(0, staking_address)
    assert_code(msg, 0)
    msg = client.ppos.getCandidateInfo(node.node_id)
    log.info("Query the pledge of the node {}".format(msg))

    assert msg["Ret"]["Shares"] == client.staking_amount + economic.add_staking_limit * 3
    assert msg["Ret"]["Released"] == client.staking_amount
    assert msg["Ret"]["RestrictingPlan"] == economic.add_staking_limit
    assert msg["Ret"]["RestrictingPlanHes"] == economic.add_staking_limit
    block_reward, staking_reward = economic.get_current_year_reward(node)

    log.info("Node 2 initiates revocation pledge")
    msg = client.staking.withdrew_staking(staking_address)
    assert_code(msg, 0)
    balance2 = node.eth.getBalance(staking_address)
    log.info("The second cycle initiated the revocation of the balance{}".format(balance2))
    """ The current increase in free funds has been withdrawn, and the following is reduced to a fee"""
    assert client.amount - balance2 - client.staking_amount < Web3.toWei(1, "ether")
    locked_info = client.ppos.getRestrictingInfo(staking_address)
    log.info("Query the lockout plan after the second cycle initiated revocation {}".format(locked_info))
    assert_code(locked_info, 0)
    assert locked_info["Ret"][
               "Pledge"] == economic.add_staking_limit, "The amount in the lockout plan is expected to be the lockout period amount."

    msg = client.ppos.getCandidateInfo(node.node_id)
    log.info("Query the pledge of node {}".format(msg))

    assert msg["Ret"]["ReleasedHes"] == 0, "Expected amount of hesitation has been refunded"
    assert msg["Ret"][
               "RestrictingPlanHes"] == 0, "Expected lockout amount has been refunded during the hesitation period"

    log.info("Enter the 3rd cycle")
    economic.wait_settlement_blocknum(node)
    balance3 = node.eth.getBalance(staking_address)
    log.info("The balance after launching the revocation in the third cycle{}".format(balance3))

    log.info("Enter the 4th cycle")
    economic.wait_settlement_blocknum(node, 1)
    balance4 = node.eth.getBalance(staking_address)
    log.info("The balance after the revocation of the second cycle {}".format(balance4))

    locked_info = client.ppos.getRestrictingInfo(staking_address)
    assert_code(locked_info, 1)

    msg = client.ppos.getCandidateInfo(node.node_id)
    log.info("Query the pledge of the node{}".format(msg))
    assert_code(msg, 301204)

    """Compute Block Reward + Pledge Reward"""
    log.info("The following is the number of blocks to get the node")
    block_number = get_block_count_number(node, economic.settlement_size * 3)
    sum_block_reward = calculate(block_reward, block_number)
    reward_sum = sum_block_reward + staking_reward
    log.info("Total amount of reward {}".format(reward_sum))

    assert client.amount + reward_sum - balance4 < Web3.toWei(1,
                                                              "ether"), "After the expected result unlock period, the money has been refunded + the block reward + pledge reward"


@allure.title("发起撤销质押（质押金+增持金额））")
@pytest.mark.P1
def test_RV_009(staking_client):
    client = staking_client
    node = client.node
    staking_address = client.staking_address
    economic = client.economic
    value_before = client.amount
    log.info("Initiate the balance before the pledge {}".format(value_before))

    log.info("Enter the second billing cycle, increase the amount")
    economic.wait_settlement_blocknum(node)
    client.staking.increase_staking(0, staking_address)
    value2 = node.eth.getBalance(staking_address)
    log.info("Pledged + increased balance {}".format(value2))
    log.info("Enter the third billing cycle, the node initiates a return")
    economic.wait_settlement_blocknum(node)
    value3 = node.eth.getBalance(staking_address)
    log.info("Balance of the 3rd cycle {}".format(value3))
    client.staking.withdrew_staking(staking_address)
    log.info("Enter the 4th billing cycle")
    economic.wait_settlement_blocknum(node)
    value4 = node.eth.getBalance(staking_address)
    log.info("The balance of the 4th billing cycle (including the reward for the 3rd cycle){}".format(value4))
    log.info("Enter the 5th billing cycle")
    economic.wait_settlement_blocknum(node)
    value5 = node.eth.getBalance(staking_address)
    log.info("Return to the pledge + overweight balance after the unlock period:{}".format(value5))
    log.info(value5 - value_before)
    amount_sum = client.staking_amount + economic.add_staking_limit
    assert value5 > value_before, "Out of the block reward exception"
    assert value5 > amount_sum, "The balance of the unlocking period is greater than the balance of the lockout period + pledge + overweight, but an exception occurs."


@pytest.mark.P2
def test_RV_011(staking_client):
    client = staking_client
    node = client.node
    economic = client.economic
    staking_address = client.staking_address
    log.info("Enter the next cycle")
    economic.wait_settlement_blocknum(node)
    log.info("Enter the next consensus round")
    economic.wait_consensus_blocknum(node)

    validator_list = get_pledge_list(node.ppos.getValidatorList)
    log.info("Consensus certifier list:{}".format(validator_list))
    assert node.node_id in validator_list
    msg = client.staking.withdrew_staking(staking_address)
    assert_code(msg, 0)


@allure.title("撤销5种身份候选人，验证人，共识验证人，不存在的候选人，已失效的候选人")
@pytest.mark.P1
def test_RV_012(client_new_node_obj_list):
    """
    Since other use cases have verified the amount of retaluation, no assertion is made here.
    0: Candidate
    1: verifier
    2: Consensus certifier
    3: Candidates that do not exist
    4: Cancelled candidate
    """
    client_a = client_new_node_obj_list[0]
    node_a = client_a.node
    client_b = client_new_node_obj_list[1]
    node_b = client_b.node
    amount_a = client_b.economic.create_staking_limit * 6
    amount_b = client_b.economic.create_staking_limit * 7
    amount = Web3.toWei(amount_a + amount_b + 10, "ether")
    address, _ = client_a.economic.account.generate_account(node_a.web3, amount)
    msg = client_a.staking.create_staking(0, address, address, amount=amount_a)

    assert_code(msg, 0)
    msg = client_b.staking.create_staking(0, address, address, amount=amount_b)

    assert_code(msg, 0)
    log.info("Enter the next cycle")
    client_b.economic.wait_settlement_blocknum(node_b)
    msg = client_b.staking.withdrew_staking(address)
    assert_code(msg, 0)


@pytest.mark.P2
def test_RV_013(staking_client):
    client = staking_client
    staking_address = client.staking_address
    node = client.node
    economic = client.economic
    log.info("Enter the next cycle")
    economic.wait_settlement_blocknum(node, 1)
    verifier_list = get_pledge_list(node.ppos.getVerifierList)
    log.info(log.info("Current billing cycle certifier {}".format(verifier_list)))
    assert node.node_id in verifier_list
    msg = client.staking.withdrew_staking(staking_address)
    assert_code(msg, 0)


@allure.title("退出验证人后，返回质押金+出块奖励+质押奖励")
@pytest.mark.P1
def test_RV_014_015(staking_client):
    """
    After becoming a verifier, there are pledge rewards and block rewards.
    """
    client = staking_client
    staking_address = client.staking_address
    node = client.node
    economic = client.economic
    economic.wait_settlement_blocknum(node)
    log.info("Enter the next cycle")
    block_reward, staking_reward = economic.get_current_year_reward(node)
    msg = client.staking.withdrew_staking(staking_address)
    log.info(msg)
    balance_1 = node.eth.getBalance(staking_address)
    log.info(balance_1)
    log.info("Enter the next cycle")
    economic.wait_settlement_blocknum(node, 2)
    balance_2 = node.eth.getBalance(staking_address)
    log.info(balance_2)
    verifier_list = get_pledge_list(node.ppos.getVerifierList)
    log.info("Current certifier list:{}".format(verifier_list))
    validator_list = get_pledge_list(node.ppos.getValidatorList)
    log.info("Current consensus certifier list:{}".format(validator_list))
    block_number = get_block_count_number(node, economic.settlement_size * 3)
    sum_block_reward = calculate(block_reward, block_number)
    reward_sum = sum_block_reward + staking_reward
    log.info("Total amount of reward {}".format(reward_sum))
    assert balance_1 + reward_sum + client.staking_amount == balance_2, "The bonus amount is abnormal"


@pytest.mark.P2
def test_RV_016(staking_client):
    _, node_id = generate_key()
    msg = staking_client.staking.withdrew_staking(staking_client.staking_address, node_id=node_id)
    log.info(msg)
    assert_code(msg, 301102)


@pytest.mark.P2
def test_RV_017(staking_client):
    client = staking_client
    node = client.node
    staking_address = client.staking_address
    msg = client.staking.withdrew_staking(staking_address)
    assert_code(msg, 0)
    msg = node.ppos.getCandidateInfo(node.node_id)
    assert msg[
               "Ret"] == "Query candidate info failed:Candidate info is not found", "Expected pledge to be successful; pledge information is deleted"
    msg = client.staking.withdrew_staking(staking_address)
    assert_code(msg, 301102)


@allure.title("修改节点收益地址，再做退回：验证质押奖励+出块奖励")
@pytest.mark.P0
def test_RV_019(staking_client):
    """
    修改钱包地址，更改后的地址收益正常
    """
    client = staking_client
    node = client.node
    staking_address = client.staking_address
    economic = client.economic
    ben_address, _ = economic.account.generate_account(node.web3)
    log.info("ben address balance:{}".format(node.eth.getBalance(ben_address)))
    log.info("Modify node information")
    msg = client.staking.edit_candidate(staking_address, ben_address)
    assert_code(msg, 0)

    log.info("Enter the second billing cycle")
    economic.wait_settlement_blocknum(node)

    block_reward, staking_reward = economic.get_current_year_reward(node)
    msg = client.staking.withdrew_staking(staking_address)
    assert_code(msg, 0)
    balance_before = node.eth.getBalance(ben_address)
    log.info("Exit the new wallet balance after pledge:{}".format(balance_before))
    log.info("Enter the third billing cycle")
    economic.wait_settlement_blocknum(node, 2)

    balance_after = node.eth.getBalance(ben_address)
    log.info("Balance after the new wallet unlock period {}".format(balance_after))

    """Compute Block Reward + Pledge Reward"""
    log.info("The following is the number of blocks to get the node")
    block_number = get_block_count_number(node, economic.settlement_size * 3)
    sum_block_reward = calculate(block_reward, block_number)
    reward_sum = sum_block_reward + staking_reward
    log.info("Total amount of reward {}".format(reward_sum))
    assert balance_after == reward_sum, "Expected new wallet balance == earnings reward"


@allure.title("退出中修改质押信息")
@pytest.mark.P2
def test__RV_020(staking_client):
    """
    Modify the pledge information in the exit
    """
    node_name = "wuyiqin"
    client = staking_client
    staking_address = client.staking_address
    node = client.node
    economic = client.economic
    economic.wait_settlement_blocknum(node)
    msg = client.staking.withdrew_staking(staking_address)
    log.info(msg)
    msg = node.ppos.getCandidateInfo(node.node_id)
    log.info(msg)
    log.info("Modify node information")
    client.staking.cfg.node_name = node_name
    msg = client.staking.edit_candidate(staking_address, staking_address)
    assert_code(msg, 301103)


@allure.title("已退出修改质押信息")
@pytest.mark.P2
def test_RV_021(staking_client):
    """
    Revoked modify pledge information
    """
    node_name = "wuyiqin"
    client = staking_client
    staking_address = client.staking_address
    node = client.node
    economic = client.economic
    economic.wait_settlement_blocknum(node)
    msg = client.staking.withdrew_staking(staking_address)
    log.info(msg)
    msg = node.ppos.getCandidateInfo(node.node_id)
    log.info(msg)
    economic.wait_settlement_blocknum(node, 2)
    log.info("Modify node information")
    client.staking.cfg.node_name = node_name
    msg = client.staking.edit_candidate(staking_address, staking_address)
    assert_code(msg, 301102)


@allure.title("最高惩罚后,返回金额&重新质押、委托、赎回")
@pytest.mark.P1
def test_RV_023(staking_client, global_test_env):
    """
    Return amount after the highest penalty
    """
    other_node = global_test_env.get_rand_node()
    client = staking_client
    staking_address = client.staking_address
    node = client.node
    economic = client.economic
    balance = node.eth.getBalance(staking_address)
    log.info(balance)
    candidate_info = client.ppos.getCandidateInfo(node.node_id)
    log.info(candidate_info)
    log.info("Stop the new verifier node")
    node.stop()
    log.info("Go to the next billing cycle")
    economic.wait_settlement_blocknum(other_node)
    msg = get_pledge_list(other_node.ppos.getCandidateList)
    log.info("Real-time certifier list {}".format(msg))
    msg = get_pledge_list(other_node.ppos.getVerifierList)
    log.info("Current billing cycle certifier {}".format(msg))
    msg = get_pledge_list(other_node.ppos.getValidatorList)
    log.info("Current consensus round certifier {}".format(msg))
    log.info("Go to the next billing cycle")
    economic.wait_settlement_blocknum(other_node, 1)
    msg = get_pledge_list(other_node.ppos.getCandidateList)
    log.info("Real-time certifier list {}".format(msg))
    verifier_list = get_pledge_list(other_node.ppos.getVerifierList)
    log.info("Current billing cycle certifier {}".format(verifier_list))
    assert node.node_id not in verifier_list, "Expected to opt out of certifier list"
    balance_before = other_node.eth.getBalance(staking_address)
    log.info("Query the account balance after being punished: {}".format(balance_before))
    log.info("Go to the next billing cycle")
    economic.wait_settlement_blocknum(other_node, 1)
    time.sleep(10)
    balance_after = other_node.eth.getBalance(staking_address)
    log.info("The balance after the penalty is refunded to the account:{}".format(balance_after))
    assert balance_before + candidate_info["Ret"][
        "Released"] == balance_after, "After being sent out and removed from the certifier, the amount is refunded abnormally"
    msg = other_node.ppos.getCandidateInfo(node.node_id)
    log.info(msg)
    node.start()
    time.sleep(10)
    staking_result = client.staking.create_staking(0, staking_address, staking_address)
    assert_code(staking_result, 0)
    candidate_info = node.ppos.getCandidateInfo(node.node_id)
    log.info(candidate_info)
    staking_blocknum = candidate_info["Ret"]["StakingBlockNum"]
    log.info("Delegation")
    msg = client.delegate.delegate(0, client.delegate_address, node.node_id)
    assert_code(msg, 0)
    msg = client.delegate.withdrew_delegate(staking_blocknum, client.delegate_address, node.node_id)
    assert_code(msg, 0)


@allure.title("退出质押后锁定期不能增持和委托")
@pytest.mark.P1
def test_RV_024(staking_client):
    """
    Can not increase and entrust after exiting pledge
    """
    client = staking_client
    node = client.node
    staking_address = client.staking_address
    economic = client.economic
    log.info("Entering the lockout period")
    economic.wait_settlement_blocknum(node)
    log.info("Node exit pledge")
    client.staking.withdrew_staking(staking_address)
    log.info("Node to increase holding")
    msg = client.staking.increase_staking(0, staking_address, amount=economic.add_staking_limit)
    assert_code(msg, 301103)
    log.info("Node to commission")
    msg = client.delegate.delegate(0, client.delegate_address)
    assert_code(msg, 301103)
