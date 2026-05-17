import { NavLink, useSearchParams } from 'react-router-dom';

const MerchantConsoleNav = () => {
  const [searchParams] = useSearchParams();
  const suffix = searchParams.toString() ? `?${searchParams.toString()}` : '';

  return (
    <div className="merchant-console-nav">
      <NavLink to={`/merchant${suffix}`} end>
        总览
      </NavLink>
      <NavLink to={`/merchant/products${suffix}`}>商品</NavLink>
      <NavLink to={`/merchant/orders${suffix}`}>订单</NavLink>
    </div>
  );
};

export default MerchantConsoleNav;
