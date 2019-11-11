import time
import pytest
import allure
from dacite import from_dict
from common.key import get_pub_key, mock_duplicate_sign
from common.log import log
from client_sdk_python import Web3
from decimal import Decimal
from tests.lib import EconomicConfig, Genesis, StakingConfig, Staking, check_node_in_list, assert_code, von_amount, \
    get_governable_parameter_value


@pytest.mark.P0
def test_LS_FV_001(client_consensus_obj):
    """
    查看锁仓账户计划
    :param client_consensus_obj:
    :return:
    """
    # Reset environment
    client_consensus_obj.economic.env.deploy_all()
    # view Lock in contract amount
    lock_up_amount = client_consensus_obj.node.eth.getBalance(EconomicConfig.FOUNDATION_LOCKUP_ADDRESS)
    log.info("Lock in contract amount: {}".format(lock_up_amount))
    # view Lockup plan
    result = client_consensus_obj.ppos.getRestrictingInfo(EconomicConfig.INCENTIVEPOOL_ADDRESS)
    release_plans_list = result['Ret']['plans']
    assert_code(result, 0)
    log.info("Lockup plan information: {}".format(result))
    # assert louck up amount
    for i in release_plans_list:
        print("a", type(release_plans_list[i]))
        print("b", EconomicConfig.release_info[i])
        assert release_plans_list[i] == EconomicConfig.release_info[
            i], "Year {} Height of block to be released: {} Release amount: {}".format(i + 1, release_plans_list[i][
            'blockNumber'], release_plans_list[i]['amount'])


def create_restrictingplan(client_new_node_obj, epoch, amount, multiple=2):
    # create restricting plan
    address, _ = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                       client_new_node_obj.economic.create_staking_limit * multiple)
    benifit_address, _ = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                               client_new_node_obj.node.web3.toWei(1000,
                                                                                                                   'ether'))
    plan = [{'Epoch': epoch, 'Amount': client_new_node_obj.node.web3.toWei(amount, 'ether')}]
    result = client_new_node_obj.restricting.createRestrictingPlan(benifit_address, plan, address)
    return result, address, benifit_address


@pytest.mark.P1
def test_LS_PV_001(client_new_node_obj):
    """
    锁仓参数的有效性验证:
                    None,
                    ""
    :param client_new_node_obj:
    :return:
    """
    # create restricting plan
    address, _ = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                       client_new_node_obj.economic.create_staking_limit)
    plan = [{'Epoch': 1, 'Amount': None}]
    try:
        result = client_new_node_obj.restricting.createRestrictingPlan(address, plan, address)
        assert_code(result, 304011)
    except Exception as e:
        log.info("Use case success, exception information：{} ".format(str(e)))

    # create restricting plan
    address, _ = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                       client_new_node_obj.economic.create_staking_limit)
    plan = [{'Epoch': 1, 'Amount': ""}]
    result = client_new_node_obj.restricting.createRestrictingPlan(address, plan, address)
    assert_code(result, 304011)


@pytest.mark.P1
def test_LS_PV_003(client_new_node_obj):
    """
    正常创建锁仓计划
    :param client_new_node_obj:
    :return:
    """
    result, address, benifit_address = create_restrictingplan(client_new_node_obj, 1, 1000)
    assert_code(result, 0)
    restricting_info = client_new_node_obj.ppos.getRestrictingInfo(benifit_address)
    assert_code(restricting_info, 0)
    assert restricting_info['Ret']['balance'] == client_new_node_obj.node.web3.toWei(1000, 'ether')


@pytest.mark.P1
@pytest.mark.parametrize('epoch, amount', [(0.1, 10), (1, 0.1)])
def test_LS_PV_004_1(client_new_node_obj, epoch, amount):
    """
    锁仓参数的有效性验证:
                    number 0.1, amount 10
                    number 1, amount 0.1
    :param client_new_node_obj:
    :return:
    """
    try:
        result, address, benifit_address = create_restrictingplan(client_new_node_obj, epoch, amount)
        assert_code(result, 0)
    except Exception as e:
        log.info("Use case success, exception information：{} ".format(str(e)))


@pytest.mark.parametrize('epoch, amount', [(-1, 10), (1, -10)])
@pytest.mark.P1
def test_LS_PV_004_2(client_new_node_obj, epoch, amount):
    """
    锁仓参数的有效性验证:epoch -1, amount 10
                      epoch 1, amount -10
    :param client_new_node_obj:
    :return:
    """
    # create restricting plan
    address, _ = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                       client_new_node_obj.economic.create_staking_limit)
    plan = [{'Epoch': epoch, 'Amount': amount}]
    try:
        result = client_new_node_obj.restricting.createRestrictingPlan(address, plan, address)
        assert_code(result, 304011)
    except Exception as e:
        log.info("Use case success, exception information：{} ".format(str(e)))


@pytest.mark.P1
def test_LS_PV_005(client_new_node_obj):
    """
    锁仓参数的有效性验证:epoch 0, amount 10
    :param client_new_node_obj:
    :return:
    """
    result, address, benifit_address = create_restrictingplan(client_new_node_obj, 0, 10)
    assert_code(result, 304001)


@pytest.mark.P1
@pytest.mark.parametrize('number', [1, 5, 36])
def test_LS_PV_006(client_new_node_obj, number):
    """
    创建锁仓计划1<= 释放计划个数N <=36
    :param client_new_node_obj:
    :return:
    """
    # create restricting plan
    address, _ = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                       client_new_node_obj.economic.create_staking_limit)
    plan = []
    for i in range(number):
        plan.append({'Epoch': i + 1, 'Amount': client_new_node_obj.node.web3.toWei(10, 'ether')})
    log.info("Create lock plan parameters：{}".format(plan))
    result = client_new_node_obj.restricting.createRestrictingPlan(address, plan, address)
    assert_code(result, 0)


@pytest.mark.P1
def test_LS_PV_007(client_new_node_obj):
    """
    创建锁仓计划-释放计划的锁定期个数 > 36
    :param client_new_node_obj:
    :return:
    """
    # create restricting plan
    address, _ = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                       client_new_node_obj.economic.create_staking_limit)
    plan = []
    for i in range(37):
        plan.append({'Epoch': i + 1, 'Amount': client_new_node_obj.node.web3.toWei(10, 'ether')})
    log.info("Create lock plan parameters：{}".format(plan))
    result = client_new_node_obj.restricting.createRestrictingPlan(address, plan, address)
    assert_code(result, 304002)


@pytest.mark.P1
def test_LS_PV_008(client_new_node_obj):
    """
    锁仓参数的有效性验证:epoch 1, amount 0
    :param client_new_node_obj:
    :return:
    """
    # create restricting plan
    result, address, benifit_address = create_restrictingplan(client_new_node_obj, 1, 0)
    assert_code(result, 304011)


@pytest.mark.P2
def test_LS_PV_009(client_new_node_obj):
    """
    创建锁仓计划-锁仓金额中文、特殊字符字符测试
    :param client_new_node_obj:
    :return:
    """
    # create restricting plan
    address, _ = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                       client_new_node_obj.economic.create_staking_limit)
    plan = [{'Epoch': 1, 'Amount': '测试 @！'}]
    result = client_new_node_obj.restricting.createRestrictingPlan(address, plan, address)
    assert_code(result, 304004)


@pytest.mark.P1
def test_LS_RV_001(client_new_node_obj):
    """
    创建锁仓计划-单个释放锁定期金额大于账户金额
    :param client_new_node_obj:
    :return:
    """
    # create restricting plan
    account_balance = client_new_node_obj.node.web3.toWei(1000, 'ether')
    Lock_in_amount = client_new_node_obj.node.web3.toWei(1001, 'ether')
    address, _ = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                       client_new_node_obj.node.web3.toWei(
                                                                           account_balance, 'ether'))
    plan = [{'Epoch': 1, 'Amount': client_new_node_obj.node.web3.toWei(Lock_in_amount, 'ether')}]
    result = client_new_node_obj.restricting.createRestrictingPlan(address, plan, address)
    assert_code(result, 304004)


@pytest.mark.P1
@pytest.mark.parametrize('balace1, balace2', [(0, 0), (300, 300), (500, 500), (500, 600)])
def test_LS_RV_002(client_new_node_obj, balace1, balace2):
    """
    创建锁仓计划-多个释放锁定期合计金额大于账户金额
    :param client_new_node_obj:
    :return:
    """
    # create restricting plan
    address, _ = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                       client_new_node_obj.node.web3.toWei(1000,
                                                                                                           'ether'))
    louk_up_balace1 = client_new_node_obj.node.web3.toWei(balace1, 'ether')
    louk_up_balace2 = client_new_node_obj.node.web3.toWei(balace2, 'ether')
    plan = [{'Epoch': 1, 'Amount': louk_up_balace1}, {'Epoch': 2, 'Amount': louk_up_balace2}]
    result = client_new_node_obj.restricting.createRestrictingPlan(address, plan, address)
    if 0 < balace1 + balace2 < 1000:
        assert_code(result, 0)
    elif 1000 <= balace1 + balace2:
        assert_code(result, 304004)
    else:
        assert_code(result, 304011)


def create_restricting_platn(client_new_node_obj, plan, benifit_address, address):
    """
    create restricting plan
    :param client_new_node_obj:
    :param plan:
    :param benifit_address:
    :param address:
    :return:
    """
    # create restricting
    result = client_new_node_obj.restricting.createRestrictingPlan(benifit_address, plan, address)
    assert_code(result, 0)
    # view restricting plan
    restricting_info = client_new_node_obj.ppos.getRestrictingInfo(benifit_address)
    log.info("Restricting information: {}".format(restricting_info))
    assert_code(restricting_info, 0)
    return restricting_info


@pytest.mark.P1
def test_LS_RV_003(client_new_node_obj):
    """
    创建锁仓计划-锁仓计划里两个锁仓计划的解锁期相同
    :param client_new_node_obj:
    :return:
    """
    # create account
    address, _ = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                       client_new_node_obj.node.web3.toWei(1000,
                                                                                                           'ether'))
    louk_up_balace = client_new_node_obj.node.web3.toWei(100, 'ether')
    plan = [{'Epoch': 1, 'Amount': louk_up_balace}, {'Epoch': 1, 'Amount': louk_up_balace}]
    # create restricting plan
    restricting_info = create_restricting_platn(client_new_node_obj, plan, address, address)
    # assert restricting plan
    assert restricting_info['Ret']['balance'] == louk_up_balace * 2, "ErrMsg:Restricting balance：{}".format(
        restricting_info['Ret']['balance'])
    assert restricting_info['Ret']['plans'][0][
               'blockNumber'] == client_new_node_obj.economic.get_settlement_switchpoint(
        client_new_node_obj.node), "ErrMsg:Restricting blockNumber {}".format(
        restricting_info['Ret']['plans'][0]['blockNumber'])
    assert restricting_info['Ret']['plans'][0][
               'amount'] == louk_up_balace * 2, "ErrMsg:Restricting amount {}".format(
        restricting_info['Ret']['plans'][0]['amount'])


@pytest.mark.P1
def test_LS_RV_004(client_new_node_obj):
    """
    创建锁仓计划-新建锁仓计划里两个锁仓计划的解锁期不同
    :param client_new_node_obj:
    :return:
    """
    address, _ = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                       client_new_node_obj.node.web3.toWei(1000,
                                                                                                           'ether'))

    louk_up_balace = client_new_node_obj.node.web3.toWei(100, 'ether')
    plan = [{'Epoch': 1, 'Amount': louk_up_balace}, {'Epoch': 2, 'Amount': louk_up_balace}]
    # create restricting plan
    restricting_info = create_restricting_platn(client_new_node_obj, plan, address, address)
    # assert restricting plan
    assert restricting_info['Ret']['balance'] == louk_up_balace * 2, "ErrMsg:Restricting balance：{}".format(
        restricting_info['Ret']['balance'])
    assert restricting_info['Ret']['plans'][0][
               'blockNumber'] == client_new_node_obj.economic.get_settlement_switchpoint(
        client_new_node_obj.node), "ErrMsg:Restricting blockNumber {}".format(
        restricting_info['Ret']['plans'][0]['blockNumber'])
    assert restricting_info['Ret']['plans'][0][
               'amount'] == louk_up_balace, "ErrMsg:Restricting amount {}".format(
        restricting_info['Ret']['plans'][0]['amount'])
    assert restricting_info['Ret']['plans'][1][
               'amount'] == louk_up_balace, "ErrMsg:Restricting amount {}".format(
        restricting_info['Ret']['plans'][1]['amount'])


@pytest.mark.P1
def test_LS_RV_005(client_new_node_obj):
    """
    创建锁仓计划-创建不同锁仓计划里2个相同解锁期
    :param client_new_node_obj:
    :return:
    """
    # create account
    address, _ = client_new_node_obj.economic.account.generate_account(client_new_node_obj.node.web3,
                                                                       client_new_node_obj.node.web3.toWei(1000,
                                                                                                           'ether'))

    louk_up_balace = client_new_node_obj.node.web3.toWei(100, 'ether')
    plan = [{'Epoch': 1, 'Amount': louk_up_balace}]
    # create restricting plan
    restricting_info = create_restricting_platn(client_new_node_obj, plan, address, address)
    # create restricting plan
    restricting_info = create_restricting_platn(client_new_node_obj, plan, address, address)
    # assert restricting plan
    assert restricting_info['Ret']['balance'] == louk_up_balace * 2, "ErrMsg:Restricting balance：{}".format(
        restricting_info['Ret']['balance'])
    assert restricting_info['Ret']['plans'][0][
               'blockNumber'] == client_new_node_obj.economic.get_settlement_switchpoint(
        client_new_node_obj.node), "ErrMsg:Restricting blockNumber {}".format(
        restricting_info['Ret']['plans'][0]['blockNumber'])
    assert restricting_info['Ret']['plans'][0][
               'amount'] == louk_up_balace * 2, "ErrMsg:Restricting amount {}".format(
        restricting_info['Ret']['plans'][0]['amount'])


def create_lock_release_amount(client_obj, amount1, amount2):
    # create account1
    lock_address, _ = client_obj.economic.account.generate_account(client_obj.node.web3, amount1)
    # create account2
    release_address, _ = client_obj.economic.account.generate_account(client_obj.node.web3, amount2)
    return lock_address, release_address


@pytest.mark.P1
def test_LS_RV_006(client_new_node_obj):
    """
    创建锁仓计划-不同个账户创建不同锁仓计划里有相同解锁期
    :param client_new_node_obj:
    :return:
    """
    # create account
    amount1 = client_new_node_obj.node.web3.toWei(1000, 'ether')
    amount2 = client_new_node_obj.node.web3.toWei(1000, 'ether')
    address1, address2 = create_lock_release_amount(client_new_node_obj, amount1, amount2)
    louk_up_balace = client_new_node_obj.node.web3.toWei(100, 'ether')
    plan = [{'Epoch': 1, 'Amount': louk_up_balace}, {'Epoch': 2, 'Amount': louk_up_balace}]
    # create restricting plan1
    restricting_info = create_restricting_platn(client_new_node_obj, plan, address1, address1)
    # create restricting plan2
    restricting_info = create_restricting_platn(client_new_node_obj, plan, address1, address2)
    # assert restricting plan1
    assert restricting_info['Ret']['balance'] == louk_up_balace * 4, "ErrMsg:Restricting balance：{}".format(
        restricting_info['Ret']['balance'])
    assert restricting_info['Ret']['plans'][0][
               'blockNumber'] == client_new_node_obj.economic.get_settlement_switchpoint(
        client_new_node_obj.node), "ErrMsg:Restricting blockNumber {}".format(
        restricting_info['Ret']['plans'][0]['blockNumber'])
    assert restricting_info['Ret']['plans'][0][
               'amount'] == louk_up_balace * 2, "ErrMsg:Restricting amount {}".format(
        restricting_info['Ret']['plans'][0]['amount'])
    assert restricting_info['Ret']['plans'][1][
               'amount'] == louk_up_balace * 2, "ErrMsg:Restricting amount {}".format(
        restricting_info['Ret']['plans'][1]['amount'])


@pytest.mark.P1
def test_LS_RV_007(client_new_node_obj):
    """
    创建锁仓计划-不同账户创建不同锁仓计划里有不相同解锁期
    :param client_new_node_obj:
    :return:
    """
    # create account
    amount1 = client_new_node_obj.node.web3.toWei(1000, 'ether')
    amount2 = client_new_node_obj.node.web3.toWei(1000, 'ether')
    address1, address2 = create_lock_release_amount(client_new_node_obj, amount1, amount2)
    louk_up_balace = client_new_node_obj.node.web3.toWei(100, 'ether')
    plan1 = [{'Epoch': 1, 'Amount': louk_up_balace}, {'Epoch': 2, 'Amount': louk_up_balace}]
    plan2 = [{'Epoch': 1, 'Amount': louk_up_balace}, {'Epoch': 3, 'Amount': louk_up_balace}]
    # create restricting plan1
    restricting_info = create_restricting_platn(client_new_node_obj, plan1, address1, address1)
    # create restricting plan2
    restricting_info = create_restricting_platn(client_new_node_obj, plan2, address1, address2)
    # assert restricting plan1
    assert restricting_info['Ret']['balance'] == louk_up_balace * 4, "ErrMsg:Restricting balance：{}".format(
        restricting_info['Ret']['balance'])
    assert restricting_info['Ret']['plans'][0][
               'blockNumber'] == client_new_node_obj.economic.get_settlement_switchpoint(
        client_new_node_obj.node), "ErrMsg:Restricting blockNumber {}".format(
        restricting_info['Ret']['plans'][0]['blockNumber'])
    assert restricting_info['Ret']['plans'][0][
               'amount'] == louk_up_balace * 2, "ErrMsg:Restricting amount {}".format(
        restricting_info['Ret']['plans'][0]['amount'])
    assert restricting_info['Ret']['plans'][1][
               'amount'] == louk_up_balace, "ErrMsg:Restricting amount {}".format(
        restricting_info['Ret']['plans'][1]['amount'])
    assert restricting_info['Ret']['plans'][2][
               'amount'] == louk_up_balace, "ErrMsg:Restricting amount {}".format(
        restricting_info['Ret']['plans'][2]['amount'])


def create_restricting_plan_and_staking(client, node, economic):
    # create account
    amount1 = von_amount(economic.create_staking_limit, 4)
    amount2 = client.node.web3.toWei(1000, 'ether')
    address1, address2 = create_lock_release_amount(client, amount1, amount2)
    # create Restricting Plan
    plan = [{'Epoch': 1, 'Amount': economic.create_staking_limit}]
    result = client.restricting.createRestrictingPlan(address2, plan, address1)
    assert_code(result, 0)
    # create staking
    result = client.staking.create_staking(1, address2, address2)
    assert_code(result, 0)
    # view Restricting Plan
    restricting_info1 = client.ppos.getRestrictingInfo(address2)
    log.info("restricting info: {}".format(restricting_info1))
    assert_code(restricting_info1, 0)
    info = restricting_info1['Ret']
    assert info['Pledge'] == economic.create_staking_limit, 'ErrMsg: restricting Pledge amount {}'.format(
        info['Pledge'])
    # wait settlement block
    economic.wait_settlement_blocknum(node)
    restricting_info2 = client.ppos.getRestrictingInfo(address2)
    log.info("current block: {}".format(node.block_number))
    log.info("restricting info: {}".format(restricting_info2))
    info = restricting_info2['Ret']
    assert info['debt'] == economic.create_staking_limit, 'ErrMsg: restricting debt amount {}'.format(
        info['debt'])
    return address1, address2


@pytest.mark.P1
def test_LS_RV_008(client_new_node_obj):
    """
    创建锁仓计划-锁仓欠释放金额<新增锁仓计划总金额
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj
    economic = client.economic
    node = client.node
    address1, address2 = create_restricting_plan_and_staking(client, node, economic)
    # create Restricting Plan again
    plan = [{'Epoch': 1, 'Amount': von_amount(economic.create_staking_limit, 2)}]
    result = client.restricting.createRestrictingPlan(address2, plan, address1)
    assert_code(result, 0)
    # view Restricting Plan
    restricting_info = client.ppos.getRestrictingInfo(address2)
    log.info("restricting info: {}".format(restricting_info))
    assert_code(restricting_info, 0)
    info = restricting_info['Ret']
    assert info['debt'] == 0, "rrMsg: restricting debt amount {}".format(info['debt'])


@pytest.mark.P1
def test_LS_RV_009(client_new_node_obj):
    """
    创建锁仓计划-锁仓欠释放金额>新增锁仓计划总金额
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj
    economic = client.economic
    node = client.node
    address1, address2 = create_restricting_plan_and_staking(client, node, economic)
    # create Restricting Plan again
    plan = [{'Epoch': 1, 'Amount': von_amount(economic.create_staking_limit, 0.8)}]
    result = client.restricting.createRestrictingPlan(address2, plan, address1)
    assert_code(result, 0)
    # view Restricting Plan
    restricting_info = client.ppos.getRestrictingInfo(address2)
    log.info("restricting info: {}".format(restricting_info))
    assert_code(restricting_info, 0)
    info = restricting_info['Ret']
    assert info['debt'] == economic.create_staking_limit - von_amount(economic.create_staking_limit,
                                                                      0.8), "rrMsg: restricting debt amount {}".format(
        info['debt'])


@pytest.mark.P1
def test_LS_RV_010(client_new_node_obj):
    """
    创建锁仓计划-锁仓欠释放金额=新增锁仓计划总金额
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj
    economic = client.economic
    node = client.node
    address1, address2 = create_restricting_plan_and_staking(client, node, economic)
    # create Restricting Plan again
    plan = [{'Epoch': 1, 'Amount': von_amount(economic.create_staking_limit, 1)}]
    result = client.restricting.createRestrictingPlan(address2, plan, address1)
    assert_code(result, 0)
    # view Restricting Plan
    restricting_info = client.ppos.getRestrictingInfo(address2)
    log.info("restricting info: {}".format(restricting_info))
    assert_code(restricting_info, 0)
    info = restricting_info['Ret']
    assert info['debt'] == 0, "rrMsg: restricting debt amount {}".format(info['debt'])


def create_restricting_plan_and_entrust(client, node, economic):
    # create account
    amount1 = von_amount(economic.create_staking_limit, 2)
    amount2 = client.node.web3.toWei(1000, 'ether')
    address1, address2 = create_lock_release_amount(client, amount1, amount2)
    # create Restricting Plan
    plan = [{'Epoch': 1, 'Amount': von_amount(economic.delegate_limit, 1)}]
    result = client.restricting.createRestrictingPlan(address2, plan, address1)
    assert_code(result, 0)
    # create staking
    result = client.staking.create_staking(0, address1, address1)
    assert_code(result, 0)
    # Application for Commission
    result = client.delegate.delegate(1, address2)
    assert_code(result, 0)
    # view Restricting Plan
    restricting_info = client.ppos.getRestrictingInfo(address2)
    log.info("restricting info: {}".format(restricting_info))
    assert_code(restricting_info, 0)
    info = restricting_info['Ret']
    assert info['Pledge'] == economic.delegate_limit, 'ErrMsg: restricting Pledge amount {}'.format(
        info['Pledge'])
    # wait settlement block
    economic.wait_settlement_blocknum(node)
    log.info("current block: {}".format(node.block_number))
    # view Restricting Plan
    restricting_info = client.ppos.getRestrictingInfo(address2)
    log.info("restricting info: {}".format(restricting_info))
    assert_code(restricting_info, 0)
    info = restricting_info['Ret']
    assert info['debt'] == economic.delegate_limit, 'ErrMsg: restricting debt amount {}'.format(
        info['debt'])
    return address1, address2


@pytest.mark.P1
def test_LS_RV_011(client_new_node_obj):
    """
    创建锁仓计划-锁仓委托释放后再次创建锁仓计划
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj
    economic = client.economic
    node = client.node
    address1, address2 = create_restricting_plan_and_entrust(client, node, economic)
    # create Restricting Plan again
    plan = [{'Epoch': 1, 'Amount': von_amount(economic.delegate_limit, 2)}]
    result = client.restricting.createRestrictingPlan(address2, plan, address1)
    assert_code(result, 0)
    # view Restricting Plan
    restricting_info = client.ppos.getRestrictingInfo(address2)
    log.info("restricting info: {}".format(restricting_info))
    assert_code(restricting_info, 0)
    info = restricting_info['Ret']
    assert info['debt'] == 0, "rrMsg: restricting debt amount {}".format(info['debt'])


@pytest.mark.P1
def test_LS_RV_012(client_new_node_obj_list, reset_environment):
    """
    创建锁仓计划-锁仓质押释放后被处罚再次创建锁仓计划
    :param client_new_node_obj_list:
    :return:
    """
    client1 = client_new_node_obj_list[0]
    log.info("Current linked client1: {}".format(client1.node.node_mark))
    client2 = client_new_node_obj_list[1]
    log.info("Current linked client2: {}".format(client2.node.node_mark))
    economic = client1.economic
    node = client1.node
    # create restricting plan and staking
    address1, address2 = create_restricting_plan_and_staking(client1, node, economic)
    # view
    candidate_info = client1.ppos.getCandidateInfo(node.node_id)
    pledge_amount = candidate_info['Ret']['Shares']
    log.info("pledge_amount: {}".format(pledge_amount))
    # Obtain pledge reward and block out reward
    block_reward, staking_reward = client1.economic.get_current_year_reward(node)
    log.info("block_reward: {} staking_reward: {}".format(block_reward, staking_reward))
    # Get 0 block rate penalties
    slash_blocks = get_governable_parameter_value(client1, 'SlashBlocksReward')
    log.info("Current block height: {}".format(client2.node.eth.blockNumber))
    # stop node
    client1.node.stop()
    # Waiting 2 consensus block
    client2.economic.wait_consensus_blocknum(client2.node, 2)
    log.info("Current block height: {}".format(client2.node.eth.blockNumber))
    # view verifier list
    verifier_list = client2.ppos.getVerifierList()
    log.info("Current settlement cycle verifier list: {}".format(verifier_list))
    # Amount of penalty
    punishment_amonut = int(Decimal(str(block_reward)) * Decimal(str(slash_blocks)))
    log.info("punishment_amonut: {}".format(punishment_amonut))
    # view Restricting Plan
    restricting_info = client2.ppos.getRestrictingInfo(address2)
    log.info("restricting info: {}".format(restricting_info))
    assert_code(restricting_info, 0)
    info = restricting_info['Ret']
    if punishment_amonut > pledge_amount:
        assert info['Pledge'] == 0, 'ErrMsg: restricting Pledge amount {}'.format(info['Pledge'])
    else:
        assert info['Pledge'] == pledge_amount - punishment_amonut, 'ErrMsg: restricting Pledge amount {}'.format(info['Pledge'])
    # create Restricting Plan again
    plan = [{'Epoch': 1, 'Amount': von_amount(economic.create_staking_limit, 2)}]
    result = client2.restricting.createRestrictingPlan(address2, plan, address1)
    assert_code(result, 0)
    # view Restricting Plan
    restricting_info3 = client2.ppos.getRestrictingInfo(address2)
    log.info("restricting info: {}".format(restricting_info3))
    assert_code(restricting_info3, 0)
    info = restricting_info3['Ret']
    assert info['debt'] == 0, "rrMsg: restricting debt amount {}".format(info['debt'])


@pytest.mark.P1
def test_LS_RV_013(client_new_node_obj):
    """
    同个账号锁仓给多个人
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj
    economic = client.economic
    node = client.node
    # create account
    address1, _ = economic.account.generate_account(node.web3, economic.create_staking_limit)
    address2, _ = economic.account.generate_account(node.web3, 0)
    address3, _ = economic.account.generate_account(node.web3, 0)
    # create Restricting Plan1
    plan = [{'Epoch': 1, 'Amount': economic.delegate_limit}]
    result = client.restricting.createRestrictingPlan(address2, plan, address1)
    assert_code(result, 0)
    restricting_info = client.ppos.getRestrictingInfo(address2)
    assert_code(restricting_info, 0)
    # create Restricting Plan1
    plan = [{'Epoch': 1, 'Amount': economic.delegate_limit}]
    result = client.restricting.createRestrictingPlan(address3, plan, address1)
    assert_code(result, 0)
    restricting_info = client.ppos.getRestrictingInfo(address3)
    assert_code(restricting_info, 0)


@pytest.mark.P1
def test_LS_RV_014(client_new_node_obj):
    """
    同个账号被多个人锁仓
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj
    economic = client.economic
    node = client.node
    # create account
    address1, _ = economic.account.generate_account(node.web3, von_amount(economic.create_staking_limit, 2))
    address2, _ = economic.account.generate_account(node.web3, von_amount(economic.create_staking_limit, 2))
    address3, _ = economic.account.generate_account(node.web3, node.web3.toWei(1000, 'ether'))
    # create Restricting Plan1
    plan = [{'Epoch': 1, 'Amount': economic.create_staking_limit}]
    result = client.restricting.createRestrictingPlan(address3, plan, address1)
    assert_code(result, 0)
    restricting_info = client.ppos.getRestrictingInfo(address3)
    assert_code(restricting_info, 0)
    # create Restricting Plan1
    plan = [{'Epoch': 1, 'Amount': economic.create_staking_limit}]
    result = client.restricting.createRestrictingPlan(address3, plan, address2)
    assert_code(result, 0)
    restricting_info = client.ppos.getRestrictingInfo(address3)
    assert_code(restricting_info, 0)
    return address3


@pytest.mark.P1
def test_LS_RV_015(client_new_node_obj):
    """
    使用多人锁仓金额质押
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj
    address3 = test_LS_RV_014(client)
    # create staking
    result = client.staking.create_staking(1, address3, address3)
    assert_code(result, 0)
    return address3


@pytest.mark.P1
def test_LS_RV_016(client_new_node_obj):
    """
    使用多人锁仓金额委托
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj
    economic = client.economic
    node = client.node
    address3 = test_LS_RV_014(client)
    # create account
    address4, _ = economic.account.generate_account(node.web3, von_amount(economic.create_staking_limit, 2))
    # create staking
    result = client.staking.create_staking(0, address4, address4)
    assert_code(result, 0)
    # Application for Commission
    result = client.delegate.delegate(1, address3, amount=economic.create_staking_limit)
    assert_code(result, 0)


@pytest.mark.P1
def test_LS_RV_017(client_new_node_obj):
    """
    使用多人锁仓金额增持
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj
    address3 = test_LS_RV_015(client)
    # Apply for additional pledge
    result = client.staking.increase_staking(1, address3)
    assert_code(result, 0)


@pytest.mark.P2
def test_LS_RV_018(client_new_node_obj_list, reset_environment):
    """
    验证人非正常状态下创建锁仓计划（节点退出创建锁仓）
    :param client_new_node_obj_list:
    :return:
    """
    client1 = client_new_node_obj_list[0]
    log.info("Current linked client1: {}".format(client1.node.node_mark))
    client2 = client_new_node_obj_list[1]
    log.info("Current linked client2: {}".format(client2.node.node_mark))
    economic = client1.economic
    node = client1.node
    # create account
    address1, _ = economic.account.generate_account(node.web3, von_amount(economic.create_staking_limit, 2))
    # create staking
    result = client1.staking.create_staking(0, address1, address1)
    assert_code(result, 0)
    # Waiting settlement block
    client1.economic.wait_settlement_blocknum(client1.node)
    # stop node
    client1.node.stop()
    # Waiting 2 consensus block
    client2.economic.wait_consensus_blocknum(client2.node, 2)
    log.info("Current block height: {}".format(client2.node.eth.blockNumber))
    # create Restricting Plan1
    plan = [{'Epoch': 1, 'Amount': economic.delegate_limit}]
    result = client2.restricting.createRestrictingPlan(address1, plan, address1)
    assert_code(result, 0)


def create_account_restricting_plan(client, economic, node):
    # create account
    address1, _ = economic.account.generate_account(node.web3, von_amount(economic.create_staking_limit, 2))
    address2, _ = economic.account.generate_account(node.web3, node.web3.toWei(1000, 'ether'))
    # create Restricting Plan
    amount = economic.create_staking_limit
    plan = [{'Epoch': 1, 'Amount': amount}]
    result = client.restricting.createRestrictingPlan(address2, plan, address1)
    assert_code(result, 0)
    # view restricting info
    restricting_info = client.ppos.getRestrictingInfo(address2)
    log.info("restricting info: {}".format(restricting_info))
    assert_code(restricting_info, 0)
    info = restricting_info['Ret']
    assert info['balance'] == amount, 'ErrMsg: restricting balance amount {}'.format(info['balance'])
    assert info['Pledge'] == 0, 'ErrMsg: restricting Pledge amount {}'.format(info['Pledge'])
    return address2


@pytest.mark.P1
def test_LS_PV_001(client_new_node_obj):
    """
    锁仓账户质押正常节点
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj
    economic = client.economic
    node = client.node
    # create account restricting plan
    address2 = create_account_restricting_plan(client, economic, node)
    # create staking
    result = client.staking.create_staking(1, address2, address2)
    assert_code(result, 0)
    # view restricting info
    restricting_info = client.ppos.getRestrictingInfo(address2)
    log.info("restricting info: {}".format(restricting_info))
    assert_code(restricting_info, 0)
    info = restricting_info['Ret']
    assert info['Pledge'] == economic.create_staking_limit, 'ErrMsg: restricting Pledge amount {}'.format(info['Pledge'])


@pytest.mark.P1
def test_LS_PV_002(client_new_node_obj):
    """
    创建计划质押-未找到锁仓信息
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj
    economic = client.economic
    node = client.node
    # create account
    address1, _ = economic.account.generate_account(node.web3, von_amount(economic.create_staking_limit, 2))
    address2, _ = economic.account.generate_account(node.web3, node.web3.toWei(1000, 'ether'))
    # create staking
    result = client.staking.create_staking(1, address2, address2)
    assert_code(result, 304005)


@pytest.mark.P1
def test_LS_PV_003(client_new_node_obj):
    """
    创建计划质押-锁仓计划质押金额<0
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj
    economic = client.economic
    node = client.node
    # create account restricting plan
    address2 = create_account_restricting_plan(client, economic, node)
    try:
        # create staking
        result = client.staking.create_staking(1, address2, address2, amount=-1)
        assert_code(result, 304008)
    except Exception as e:
        log.info("Use case success, exception information：{} ".format(str(e)))


@pytest.mark.P1
def test_LS_PV_004(client_new_node_obj):
    """
    创建计划质押-锁仓计划质押金额=0
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj
    economic = client.economic
    node = client.node
    # create account restricting plan
    address2 = create_account_restricting_plan(client, economic, node)
    # create staking
    result = client.staking.create_staking(1, address2, address2, amount=0)
    assert_code(result, 304007)


@pytest.mark.P1
def test_LS_PV_005(client_new_node_obj):
    """
    创建计划质押-锁仓计划质押金额小于最低门槛
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj
    economic = client.economic
    node = client.node
    # create account restricting plan
    address2 = create_account_restricting_plan(client, economic, node)
    # create staking
    staking_amount = von_amount(economic.create_staking_limit, 0.8)
    result = client.staking.create_staking(1, address2, address2, amount=staking_amount)
    assert_code(result, 301100)


@pytest.mark.P2
def test_LS_PV_006(client_new_node_obj):
    """
    创建计划质押-锁仓账户余额为0的情况下申请质押
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj
    economic = client.economic
    node = client.node
    # create account
    address1, _ = economic.account.generate_account(node.web3, von_amount(economic.create_staking_limit, 2))
    address2, _ = economic.account.generate_account(node.web3, 0)
    # create Restricting Plan
    amount = economic.create_staking_limit
    plan = [{'Epoch': 1, 'Amount': amount}]
    result = client.restricting.createRestrictingPlan(address2, plan, address1)
    assert_code(result, 0)
    try:
        # create staking
        result = client.staking.create_staking(1, address2, address2)
        assert_code(result, 304005)
    except Exception as e:
        log.info("Use case success, exception information：{} ".format(str(e)))


@pytest.mark.P1
def test_LS_PV_007(client_new_node_obj_list):
    """
    创建计划退回质押-退回质押金额>锁仓质押金额
    :param client_new_node_obj_list:
    :return:
    """
    client1 = client_new_node_obj_list[0]
    log.info("Current linked client1: {}".format(client1.node.node_mark))
    client2 = client_new_node_obj_list[1]
    log.info("Current linked client2: {}".format(client2.node.node_mark))
    economic = client1.economic
    node = client1.node
    # create account
    amount1 = von_amount(economic.create_staking_limit, 2)
    amount2 = von_amount(economic.create_staking_limit, 2)
    address1, address2 = create_lock_release_amount(client1, amount1, amount2)
    # create Restricting Plan
    plan = [{'Epoch': 1, 'Amount': economic.create_staking_limit}]
    result = client1.restricting.createRestrictingPlan(address2, plan, address1)
    assert_code(result, 0)
    # create Restricting amount staking
    result = client1.staking.create_staking(1, address2, address2)
    assert_code(result, 0)
    time.sleep(3)
    # create Free amount staking
    result = client2.staking.create_staking(0, address2, address2)
    assert_code(result, 0)
    # withdrew staking
    result = client2.staking.withdrew_staking(address2)
    assert_code(result, 0)


@pytest.mark.P1
def test_LS_PV_008(client_new_node_obj):
    """
    创建计划退回质押-欠释放金额=回退金额
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj
    economic = client.economic
    node = client.node
    # create restricting plan and staking
    address1, address2 = create_restricting_plan_and_staking(client, economic, node)
    # withdrew staking
    result = client.staking.withdrew_staking(address2)
    assert_code(result, 0)


@pytest.mark.P1
def test_LS_PV_009(client_new_node_obj_list):
    """
    创建计划退回质押-欠释放金额<回退金额
    :param client_new_node_obj_list:
    :return:
    """
    client1 = client_new_node_obj_list[0]
    log.info("Current linked client1: {}".format(client1.node.node_mark))
    client2 = client_new_node_obj_list[1]
    log.info("Current linked client2: {}".format(client2.node.node_mark))
    economic = client1.economic
    node = client1.node
    # create account
    amount1 = von_amount(economic.create_staking_limit, 2)
    amount2 = von_amount(economic.create_staking_limit, 2)
    address1, address2 = create_lock_release_amount(client1, amount1, amount2)
    # create Restricting Plan
    plan = [{'Epoch': 1, 'Amount': economic.create_staking_limit}]
    result = client1.restricting.createRestrictingPlan(address2, plan, address1)
    assert_code(result, 0)
    # create Restricting amount staking
    result = client1.staking.create_staking(1, address2, address2)
    assert_code(result, 0)
    # wait settlement block
    economic.wait_settlement_blocknum(node)
    # view restricting info
    restricting_info = client1.ppos.getRestrictingInfo(address2)
    info = restricting_info['Ret']
    assert info['dept'] == economic.create_staking_limit, "rrMsg: restricting debt amount {}".format(info['debt'])
    # create Free amount staking
    result = client2.staking.create_staking(0, address2, address2)
    assert_code(result, 0)
    # withdrew staking
    result = client2.staking.withdrew_staking(address2)
    assert_code(result, 0)
    # view Restricting plan
    restricting_info = client2.ppos.getRestrictingInfo(address2)
    assert_code(restricting_info, 0)
    info = restricting_info['Ret']
    assert info['debt'] == 0, "errMsg: restricting debt amount {}".format(info['debt'])


@pytest.mark.P2
def test_LS_PV_010(client_new_node_obj):
    """
    创建计划退回质押-锁仓账户余额不足的情况下申请退回质押
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj
    economic = client.economic
    node = client.node
    # create account
    amount1 = von_amount(economic.create_staking_limit, 2)
    amount2 = node.web3.toWei(0.000009, 'ether')
    address1, address2 = create_lock_release_amount(client, amount1, amount2)
    # create Restricting Plan
    plan = [{'Epoch': 1, 'Amount': economic.create_staking_limit}]
    result = client.restricting.createRestrictingPlan(address2, plan, address1)
    assert_code(result, 0)
    # create Restricting amount staking
    result = client.staking.create_staking(1, address2, address2)
    assert_code(result, 0)
    log.info("address amount: {}".format(node.eth.getBalance(address2)))
    try:
        # withdrew staking
        result = client.staking.withdrew_staking(address2)
        assert_code(result, 0)
    except Exception as e:
        log.info("Use case success, exception information：{} ".format(str(e)))


@pytest.mark.P2
def test_LS_PV_011(client_new_node_obj):
    """
    锁仓账户退回质押金中，申请质押节点
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj
    economic = client.economic
    node = client.node
    # create restricting plan and staking
    address1, address2 = create_restricting_plan_and_staking(client, node, economic)
    # withdrew staking
    result = client.staking.withdrew_staking(address2)
    assert_code(result, 0)
    # create Restricting amount staking
    result = client.staking.create_staking(1, address2, address2)
    assert_code(result, 301101)


@pytest.mark.P2
def test_LS_PV_012(client_new_node_obj):
    """
    锁仓账户申请完质押后又退回质押金（犹豫期）
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj
    economic = client.economic
    node = client.node
    # create account restricting plan
    address2 = create_account_restricting_plan(client, economic, node)
    # create staking
    staking_amount = von_amount(economic.create_staking_limit)
    result = client.staking.create_staking(1, address2, address2, amount=staking_amount)
    assert_code(result, 0)
    # withdrew staking
    result = client.staking.withdrew_staking(address2)
    assert_code(result, 0)


@pytest.mark.P1
def test_LS_PV_013(client_new_node_obj):
    """
    锁仓账户申请完质押后又退回质押金（锁定期）
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj
    economic = client.economic
    node = client.node
    # create account restricting plan
    address2 = create_account_restricting_plan(client, economic, node)
    # create staking
    result = client.staking.create_staking(1, address2, address2)
    assert_code(result, 0)
    # wait settlement block
    economic.wait_settlement_blocknum(node)
    # withdrew staking
    result = client.staking.withdrew_staking(address2)
    assert_code(result, 0)
    # wait settlement block
    economic.wait_settlement_blocknum(node, 2)
    # view restricting info
    restricting_info = client.ppos.getRestrictingInfo(address2)
    log.info("restricting info: {}".format(restricting_info))
    assert_code(restricting_info, 304005)


@pytest.mark.P1
def test_LS_EV_001(client_new_node_obj):
    """
    创建计划委托-委托正常节点
    :param client_new_node_obj:
    :return:
    """
    client = client_new_node_obj
    economic = client.economic
    node = client.node
    # create account
    amount1 = von_amount(economic.create_staking_limit, 2)
    amount2 = client.node.web3.toWei(1000, 'ether')
    address1, address2 = create_lock_release_amount(client, amount1, amount2)
    # create Restricting Plan
    plan = [{'Epoch': 1, 'Amount': von_amount(economic.delegate_limit, 1)}]
    result = client.restricting.createRestrictingPlan(address2, plan, address1)
    assert_code(result, 0)
    # create staking
    result = client.staking.create_staking(0, address1, address1)
    assert_code(result, 0)
    # Application for Commission
    result = client.delegate.delegate(1, address2)
    assert_code(result, 0)
    # view Restricting Plan
    restricting_info = client.ppos.getRestrictingInfo(address2)
    log.info("restricting info: {}".format(restricting_info))
    assert_code(restricting_info, 0)
    info = restricting_info['Ret']
    assert info['Pledge'] == economic.delegate_limit, 'ErrMsg: restricting Pledge amount {}'.format(
        info['Pledge'])






