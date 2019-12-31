package versioncompatible.v0_4_25;

import beforetest.ContractPrepareTest;
import com.alibaba.fastjson.JSONObject;
import network.platon.autotest.junit.annotations.DataSource;
import network.platon.autotest.junit.enums.DataSourceType;
import network.platon.contracts.ContractAndAddressFunction;
import org.apache.commons.lang.StringUtils;
import org.junit.Test;
import org.web3j.protocol.core.DefaultBlockParameterName;
import org.web3j.protocol.core.methods.response.PlatonGetBalance;
import org.web3j.protocol.core.methods.response.TransactionReceipt;
import org.web3j.tuples.generated.Tuple3;
import org.web3j.tx.Transfer;
import org.web3j.utils.Convert;

import java.math.BigDecimal;
import java.math.BigInteger;
/**
 * @title 0.4.25版本合约和地址成员变量/函数测试
 * 1.0.4.25版本contract合约类型包括 address类型的成员函数，可以直接使用 send()成员函数验证
 * 2.0.4.25版本contract合约类型包括 address类型的成员函数，可以直接使用 transfer()成员函数验证
 * 3.0.4.25版本contract合约类型包括 address类型的成员函数，可以直接使用 balance成员变量验证
 * 4.0.4.25版本msg.sender类型所属验证
 * @description:
 * @author: albedo
 * @create: 2019/12/28
 */
public class ContractAndAddressFunctionTest extends ContractPrepareTest {

    static final BigInteger GAS_LIMIT = BigInteger.valueOf(990000);
    static final BigInteger GAS_PRICE = BigInteger.valueOf(1000000000L);

    @Test
    @DataSource(type = DataSourceType.EXCEL, file = "test.xls", sheetName = "Sheet1",
            author = "albedo", showName = "versioncompatible.v0_4_25.ContractAndAddressTest-合约和地址成员变量(函数)")
    public void testAddressCheck() {
        try {
            prepare();
            ContractAndAddressFunction contractAndAddress = ContractAndAddressFunction.deploy(web3j, transactionManager, provider).send();
            String contractAddress = contractAndAddress.getContractAddress();
            String transactionHash = contractAndAddress.getTransactionReceipt().get().getTransactionHash();
            collector.logStepPass("ContractAndAddress issued successfully.contractAddress:" + contractAddress + ", hash:" + transactionHash);

            Transfer transfer = new Transfer(web3j, transactionManager);
            TransactionReceipt receipt = transfer.sendFunds(contractAddress, BigDecimal.valueOf(300.00), Convert.Unit.LAT, GAS_PRICE, GAS_LIMIT).send();
            if (StringUtils.equals(receipt.getStatus(), "0x0")) {
                PlatonGetBalance balance = web3j.platonGetBalance(contractAddress, DefaultBlockParameterName.LATEST).send();
                collector.logStepPass("transfer contract account is successfully.contractAddress:" + contractAddress + ", amount:" + balance.getResult());

            }
            Tuple3<String, BigInteger, BigInteger> result = contractAndAddress.addressCheck().send();
            Tuple3<String, BigInteger, BigInteger> expert = new Tuple3<>(contractAddress, new BigInteger("0"), new BigInteger("0"));
            collector.assertEqual(JSONObject.toJSONString(result), JSONObject.toJSONString(expert), "checkout contract address function");
        } catch (Exception e) {
            e.printStackTrace();
        }
    }
}
